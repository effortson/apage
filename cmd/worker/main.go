// Command apage-worker runs async scan/convert/expiry/delete jobs
// (spec §22.2: apage-worker).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/apage/apage/internal/config"
	"github.com/apage/apage/internal/objstore"
	"github.com/apage/apage/internal/redisx"
	"github.com/apage/apage/internal/store"
	"github.com/apage/apage/internal/worker"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("postgres", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	rdb, err := redisx.New(cfg.RedisURL)
	if err != nil {
		log.Error("redis", "err", err)
		os.Exit(1)
	}
	defer rdb.Close()

	var obj worker.ObjectStore
	if o, err := objstore.New(objstore.Config{
		Endpoint: cfg.S3Endpoint, Bucket: cfg.S3Bucket, Region: cfg.S3Region,
		AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey, UseSSL: cfg.S3UseSSL,
		PresignTTL: time.Duration(cfg.PresignURLTTLSeconds) * time.Second,
	}); err != nil {
		log.Warn("object storage unavailable (delete disabled)", "err", err)
	} else {
		obj = o
	}

	w := worker.New(db, rdb, obj, log)
	log.Info("apage-worker started")
	w.Run(ctx)
	log.Info("apage-worker stopped")
}
