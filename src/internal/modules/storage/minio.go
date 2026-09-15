package storage

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinioConfig holds MinIO-specific connection parameters.
// This is the struct that callers marshal into json.RawMessage when
// calling SetClient with ProviderMinio.
type MinioConfig struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	UseSSL    bool   `json:"use_ssl"`
	// Region is optional. For AWS S3 it pins the signing region so the client
	// skips the GetBucketLocation lookup and creates buckets in the right region.
	// Leave empty for self-hosted MinIO.
	Region string `json:"region"`
	// InsecureSkipVerify should only be true in dev/test environments.
	InsecureSkipVerify bool `json:"insecure_skip_verify"`
}

// ── Provider (factory) ────────────────────────────────────────────────────────

type minioProvider struct{}

// NewMinioProvider returns the StorageProvider for MinIO.
func NewMinioProvider() StorageProvider { return &minioProvider{} }

func (p *minioProvider) Type() ProviderType { return ProviderMinio }

func (p *minioProvider) ComputeHash(rawConfig json.RawMessage) string {
	return defaultComputeHash(rawConfig)
}

func (p *minioProvider) CreateClient(rawConfig json.RawMessage) (StorageClient, error) {
	var cfg MinioConfig
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		return nil, fmt.Errorf("minio: invalid config: %w", err)
	}

	inner, err := newRawMinioClient(cfg)
	if err != nil {
		return nil, err
	}

	return &minioClient{inner: inner, region: cfg.Region}, nil
}

// ── Client (adapter) ──────────────────────────────────────────────────────────

// minioClient wraps *minio.Client and implements StorageClient.
type minioClient struct {
	inner  *minio.Client
	region string
}

// ── Core CRUD ─────────────────────────────────────────────────────────────────

func (c *minioClient) Ping(ctx context.Context) error {
	_, err := c.inner.ListBuckets(ctx)
	if err != nil {
		return fmt.Errorf("minio: ping failed: %w", err)
	}
	return nil
}

func (c *minioClient) ListBuckets(ctx context.Context) ([]BucketInfo, error) {
	buckets, err := c.inner.ListBuckets(ctx)
	if err != nil {
		return nil, fmt.Errorf("minio: list buckets: %w", err)
	}

	result := make([]BucketInfo, len(buckets))
	for i, b := range buckets {
		result[i] = BucketInfo{Name: b.Name, CreatedAt: b.CreationDate}
	}
	return result, nil
}

func (c *minioClient) PutObject(ctx context.Context, req PutObjectRequest) error {
	// req.Data is already a full in-memory buffer, so a bytes.Reader streams it
	// directly — no io.Pipe + writer goroutine, which would otherwise leak if the
	// upload's ctx is canceled mid-write and minio-go abandons the reader.
	//
	// DisableMultipart forces a single-part PUT so the object's ETag is the
	// content MD5. The artifact harvest dedups by comparing the stored ETag to
	// the file's MD5; multipart ETags ("<hash>-<parts>") would never match,
	// causing large files to be re-uploaded as a duplicate version every turn.
	// Single PUT is valid up to the 5 GiB S3 limit, which bounds our artifacts.
	_, err := c.inner.PutObject(ctx, req.Bucket, req.Key,
		bytes.NewReader(req.Data), int64(len(req.Data)),
		minio.PutObjectOptions{ContentType: req.ContentType, DisableMultipart: true})
	if err != nil {
		return fmt.Errorf("minio: put object: %w", err)
	}
	return nil
}

// PutObjectStream uploads directly from req.Reader. minio-go streams the
// reader to the wire, so heap usage stays flat regardless of object size.
// A known Size lets it choose single-part PUT (so the ETag is the content
// MD5, which the harvest's change detection relies on); Size < 0 falls back
// to multipart.
func (c *minioClient) PutObjectStream(ctx context.Context, req PutObjectStreamRequest) error {
	// Single-part PUT (when the size is known) so the ETag is the content MD5
	// — see PutObject. DisableMultipart requires a known length, so leave it
	// off for the unknown-size case (minio-go then streams as multipart).
	_, err := c.inner.PutObject(ctx, req.Bucket, req.Key, req.Reader, req.Size,
		minio.PutObjectOptions{ContentType: req.ContentType, DisableMultipart: req.Size >= 0})
	if err != nil {
		return fmt.Errorf("minio: put object stream: %w", err)
	}
	return nil
}

func (c *minioClient) GetObject(ctx context.Context, req GetObjectRequest) ([]byte, error) {
	obj, err := c.inner.GetObject(ctx, req.Bucket, req.Key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("minio: get object: %w", err)
	}
	defer obj.Close()

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, fmt.Errorf("minio: read object: %w", err)
	}
	return data, nil
}

func (c *minioClient) StatObject(ctx context.Context, bucket, key string) (ObjectStat, error) {
	info, err := c.inner.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return ObjectStat{}, fmt.Errorf("minio: stat object: %w", err)
	}
	return ObjectStat{
		Size:         info.Size,
		ContentType:  info.ContentType,
		LastModified: info.LastModified,
	}, nil
}

func (c *minioClient) DeleteObject(ctx context.Context, bucket, key string) error {
	err := c.inner.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("minio: delete object: %w", err)
	}
	return nil
}

// ── Bucket management ─────────────────────────────────────────────────────────

func (c *minioClient) BucketExists(ctx context.Context, bucket string) (bool, error) {
	exists, err := c.inner.BucketExists(ctx, bucket)
	if err != nil {
		return false, fmt.Errorf("minio: bucket exists check: %w", err)
	}
	return exists, nil
}

func (c *minioClient) CreateBucket(ctx context.Context, bucket string) error {
	err := c.inner.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: c.region})
	if err != nil {
		return fmt.Errorf("minio: create bucket: %w", err)
	}
	return nil
}

func (c *minioClient) RemoveBucket(ctx context.Context, bucket string) error {
	err := c.inner.RemoveBucket(ctx, bucket)
	if err != nil {
		return fmt.Errorf("minio: remove bucket: %w", err)
	}
	return nil
}

// ── Object listing & bulk delete ──────────────────────────────────────────────

// ListObjects streams ObjectEntry values from bucket/prefix.
// The returned channel is closed when listing is complete or ctx is cancelled.
func (c *minioClient) ListObjects(ctx context.Context, bucket, prefix string, recursive bool) (<-chan ObjectEntry, error) {
	outCh := make(chan ObjectEntry)
	go func() {
		defer close(outCh)
		for obj := range c.inner.ListObjects(ctx, bucket, minio.ListObjectsOptions{
			Prefix:    prefix,
			Recursive: recursive,
		}) {
			entry := ObjectEntry{
				Key:          obj.Key,
				Size:         obj.Size,
				LastModified: obj.LastModified,
				ContentType:  obj.ContentType,
				ETag:         strings.Trim(obj.ETag, `"`),
				IsDir:        strings.HasSuffix(obj.Key, "/"),
				Err:          obj.Err,
			}
			select {
			case outCh <- entry:
			case <-ctx.Done():
				return
			}
		}
	}()
	return outCh, nil
}

// RemoveObjects bulk-deletes the objects emitted by the input channel and
// streams any per-object errors on the returned channel.
func (c *minioClient) RemoveObjects(ctx context.Context, bucket string, objects <-chan ObjectEntry) <-chan RemoveObjectError {
	// Bridge storage.ObjectEntry channel → minio.ObjectInfo channel.
	minioObjects := make(chan minio.ObjectInfo)
	go func() {
		defer close(minioObjects)
		for entry := range objects {
			select {
			case minioObjects <- minio.ObjectInfo{Key: entry.Key}:
			case <-ctx.Done():
				return
			}
		}
	}()

	errCh := c.inner.RemoveObjects(ctx, bucket, minioObjects, minio.RemoveObjectsOptions{GovernanceBypass: true})

	// Bridge minio.RemoveObjectError → storage.RemoveObjectError.
	outCh := make(chan RemoveObjectError)
	go func() {
		defer close(outCh)
		for rErr := range errCh {
			select {
			case outCh <- RemoveObjectError{Key: rErr.ObjectName, Err: rErr.Err}:
			case <-ctx.Done():
				// Consumer abandoned the channel (e.g. returned on first
				// error); stop instead of blocking forever on an unread send.
				return
			}
		}
	}()
	return outCh
}

// ── Presigned URLs ────────────────────────────────────────────────────────────

func (c *minioClient) PresignedPutObject(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	u, err := c.inner.PresignedPutObject(ctx, bucket, key, expiry)
	if err != nil {
		return "", fmt.Errorf("minio: presigned put: %w", err)
	}
	return u.String(), nil
}

func (c *minioClient) PresignedGetObject(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	u, err := c.inner.PresignedGetObject(ctx, bucket, key, expiry, nil)
	if err != nil {
		return "", fmt.Errorf("minio: presigned get: %w", err)
	}
	return u.String(), nil
}

// ── Internal helpers ──────────────────────────────────────────────────────────

func newRawMinioClient(cfg MinioConfig) (*minio.Client, error) {
	endpoint := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.InsecureSkipVerify, //nolint:gosec // explicit user opt-in for self-signed MinIO/S3 endpoints
		},
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:     credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure:    cfg.UseSSL,
		Region:    cfg.Region,
		Transport: tr,
	})
	if err != nil {
		return nil, fmt.Errorf("minio: failed to create client: %w", err)
	}

	return client, nil
}

// TestMinioConnection validates a MinioConfig by attempting a real connection.
// Kept as a standalone helper so callers can pre-validate before calling SetClient.
func TestMinioConnection(cfg MinioConfig) error {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("minio: failed to marshal config: %w", err)
	}

	p := NewMinioProvider()
	client, err := p.CreateClient(raw)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return client.Ping(ctx)
}
