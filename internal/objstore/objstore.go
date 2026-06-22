// Package objstore implements the ObjectStore over any S3-compatible backend
// (S3, R2, MinIO, OSS, COS) via minio-go. Spec §11/§19.3: the API signs short
// upload/download URLs and does not proxy large files.
package objstore

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/minio-go/v7/pkg/lifecycle"
)

// Store is an S3-compatible object store.
type Store struct {
	client        *minio.Client
	presignClient *minio.Client // signs browser-reachable URLs (may differ from client)
	bucket        string
	presign       time.Duration
}

// Config configures the object store.
type Config struct {
	Endpoint string
	// PublicEndpoint, when set, is used to sign GET/PUT URLs that a browser can
	// reach (the main Endpoint may be internal-only). Empty => use Endpoint.
	PublicEndpoint string
	Bucket         string
	Region         string
	AccessKey      string
	SecretKey      string
	UseSSL         bool
	PresignTTL     time.Duration
	// LifecycleDays, when >0, installs a bucket lifecycle rule expiring objects
	// after N days as an orphan-cleanup backstop (spec §19.3).
	LifecycleDays int
}

// New connects to the object store and ensures the bucket exists.
func New(cfg Config) (*Store, error) {
	endpoint := cfg.Endpoint
	// minio-go wants host:port without scheme.
	if u, err := url.Parse(cfg.Endpoint); err == nil && u.Host != "" {
		endpoint = u.Host
		cfg.UseSSL = u.Scheme == "https"
	}
	cli, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	exists, err := cli.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("bucket check: %w", err)
	}
	if !exists {
		if err := cli.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{Region: cfg.Region}); err != nil {
			return nil, fmt.Errorf("make bucket: %w", err)
		}
	}
	// Orphan-cleanup backstop (spec §19.3). Best-effort: some backends/permissions
	// do not support lifecycle, which must not block startup.
	if cfg.LifecycleDays > 0 {
		lc := lifecycle.NewConfiguration()
		lc.Rules = []lifecycle.Rule{{
			ID:         "apage-orphan-backstop",
			Status:     "Enabled",
			RuleFilter: lifecycle.Filter{Prefix: ""},
			Expiration: lifecycle.Expiration{Days: lifecycle.ExpirationDays(cfg.LifecycleDays)},
		}}
		_ = cli.SetBucketLifecycle(ctx, cfg.Bucket, lc)
	}
	ttl := cfg.PresignTTL
	if ttl == 0 {
		ttl = 15 * time.Minute
	}
	// Presign against the public endpoint when configured so signed URLs are
	// browser-reachable; otherwise reuse the main client (spec §19.3).
	presignCli := cli
	if cfg.PublicEndpoint != "" {
		pubHost, pubSSL := cfg.Endpoint, cfg.UseSSL
		if u, err := url.Parse(cfg.PublicEndpoint); err == nil && u.Host != "" {
			pubHost, pubSSL = u.Host, u.Scheme == "https"
		}
		pc, err := minio.New(pubHost, &minio.Options{
			Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
			Secure: pubSSL,
			Region: cfg.Region,
		})
		if err != nil {
			return nil, fmt.Errorf("public presign client: %w", err)
		}
		presignCli = pc
	}
	return &Store{client: cli, presignClient: presignCli, bucket: cfg.Bucket, presign: ttl}, nil
}

// Put uploads an object (direct small-file path, spec §12).
func (s *Store) Put(key, contentType string, r io.Reader) error {
	_, err := s.client.PutObject(context.Background(), s.bucket, key, r, -1,
		minio.PutObjectOptions{ContentType: contentType})
	return err
}

// PresignPut returns a presigned PUT URL (spec §12: 15-min TTL). Signed against
// the public endpoint so a browser can upload directly.
func (s *Store) PresignPut(key, contentType string) (string, map[string]string, error) {
	u, err := s.presignClient.PresignedPutObject(context.Background(), s.bucket, key, s.presign)
	if err != nil {
		return "", nil, err
	}
	return u.String(), map[string]string{"Content-Type": contentType}, nil
}

// PresignGet returns a presigned GET URL with a download filename (spec §19.6).
// Signed against the public endpoint so a browser can fetch the bytes directly.
func (s *Store) PresignGet(key, downloadName string) (string, error) {
	params := url.Values{}
	if downloadName != "" {
		// Neutralize quotes/backslashes/control bytes so a hostile display name
		// cannot break out of the quoted filename in the disposition the object
		// store echoes back to the browser.
		params.Set("response-content-disposition", "inline; filename=\""+sanitizeFilename(downloadName)+"\"")
	}
	u, err := s.presignClient.PresignedGetObject(context.Background(), s.bucket, key, s.presign, params)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// sanitizeFilename strips characters that could break out of a quoted filename
// in a Content-Disposition header (quotes, backslashes, control bytes).
func sanitizeFilename(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r == 0x7f || r == '"' || r == '\\' {
			continue
		}
		b.WriteRune(r)
	}
	if b.Len() == 0 {
		return "download"
	}
	return b.String()
}

// Get opens an object for range streaming (spec §11/§13). *minio.Object
// satisfies io.ReadSeekCloser.
func (s *Store) Get(key string) (body io.ReadSeekCloser, contentType string, size int64, err error) {
	obj, err := s.client.GetObject(context.Background(), s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, "", 0, err
	}
	st, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		return nil, "", 0, err
	}
	return obj, st.ContentType, st.Size, nil
}

// Stat returns the size and ETag of a stored object, or an error if it is
// absent. Used to verify a presigned upload actually landed (spec §12 integrity).
func (s *Store) Stat(key string) (size int64, etag string, err error) {
	info, err := s.client.StatObject(context.Background(), s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return 0, "", err
	}
	return info.Size, info.ETag, nil
}

// Delete removes objects (spec §11: delete original/preview/thumb).
func (s *Store) Delete(keys ...string) error {
	for _, k := range keys {
		if err := s.client.RemoveObject(context.Background(), s.bucket, k, minio.RemoveObjectOptions{}); err != nil {
			return err
		}
	}
	return nil
}
