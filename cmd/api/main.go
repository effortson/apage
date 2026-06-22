// Command apage-api runs the REST control plane + data-plane + visitor runtime
// (spec §22.2 service split: apage-api).
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/apage/apage/internal/api"
	"github.com/apage/apage/internal/config"
	"github.com/apage/apage/internal/gatewayclient"
	"github.com/apage/apage/internal/mail"
	"github.com/apage/apage/internal/objstore"
	"github.com/apage/apage/internal/redisx"
	"github.com/apage/apage/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	ctx := context.Background()

	db, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("postgres", "err", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		log.Error("migrate", "err", err)
		os.Exit(1)
	}

	rdb, err := redisx.New(cfg.RedisURL)
	if err != nil {
		log.Error("redis", "err", err)
		os.Exit(1)
	}
	defer rdb.Close()

	obj, err := objstore.New(objstore.Config{
		Endpoint: cfg.S3Endpoint, PublicEndpoint: cfg.S3PublicEndpoint, Bucket: cfg.S3Bucket, Region: cfg.S3Region,
		AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey, UseSSL: cfg.S3UseSSL,
		PresignTTL: time.Duration(cfg.PresignURLTTLSeconds) * time.Second, LifecycleDays: cfg.S3LifecycleDays,
	})
	if err != nil {
		// Object storage is optional for tunnel-only MVP; log and continue.
		log.Warn("object storage unavailable (cloud upload disabled)", "err", err)
	}

	var mailer api.Mailer
	if cfg.SMTPHost != "" {
		mailer = mail.SMTPMailer{Host: cfg.SMTPHost, Port: cfg.SMTPPort, User: cfg.SMTPUser, Pass: cfg.SMTPPass, From: cfg.MailFrom}
	} else {
		mailer = mail.LogMailer{Log: log}
	}

	gw := gatewayclient.New(cfg.GatewayInternalURL)

	var objIface api.ObjectStore
	if obj != nil {
		objIface = obj
	}
	srv := api.New(cfg, db, rdb, log, mailer, gw, objIface)
	srv.BootstrapAdmin(ctx) // seed the first platform admin from config (spec §8)

	httpSrv := &http.Server{
		Addr:              cfg.APIAddr,
		Handler:           srv.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("apage-api listening", "addr", cfg.APIAddr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("listen", "err", err)
			os.Exit(1)
		}
	}()

	// Internal observability listener (spec §18/§31) — kept off the public host.
	metricsSrv := &http.Server{Addr: cfg.MetricsAddr, Handler: srv.MetricsHandler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Info("apage-api metrics listening", "addr", cfg.MetricsAddr)
		if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Warn("metrics listen", "err", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	_ = metricsSrv.Shutdown(shutdownCtx)
	log.Info("apage-api stopped")
}
