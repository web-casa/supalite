// Package backup wraps S3 operations for logical (pg_dump) backups.
//
// Design notes:
//   - Config is read from the process env at each request (FromEnv).
//     S3 client construction is cheap (no network in NewFromConfig),
//     so we don't bother caching. Note: docker-compose sets env vars
//     at container start, so saving new creds in the Settings UI
//     still requires restarting the admin container to take effect.
//   - All listing/deleting is scoped to a single prefix (BACKUP_S3_PREFIX,
//     default "backup/") to prevent accidental bucket traversal when
//     the bucket is shared with other workloads.
//   - Path-style addressing is required for MinIO/Ceph, and for
//     Cloudflare R2 when using custom endpoints without the account
//     subdomain.
package backup

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type Config struct {
	Endpoint    string // e.g. https://minio.example.com — empty means AWS default
	Bucket      string
	Region      string // default us-east-1
	AccessKey   string
	SecretKey   string
	PathStyle   bool   // required for MinIO; usually false for AWS/R2
	Prefix      string // default "backup/"
}

type Client struct {
	cfg       Config
	s3        *s3.Client
	presigner *s3.PresignClient
	uploader  *manager.Uploader
}

// FromEnv reads BACKUP_S3_* env vars. Returns an error if the config
// is incomplete (bucket + credentials are required).
func FromEnv() (*Config, error) {
	cfg := Config{
		Endpoint:  strings.TrimSpace(os.Getenv("BACKUP_S3_ENDPOINT")),
		Bucket:    strings.TrimSpace(os.Getenv("BACKUP_S3_BUCKET")),
		Region:    strings.TrimSpace(os.Getenv("BACKUP_S3_REGION")),
		AccessKey: strings.TrimSpace(os.Getenv("BACKUP_S3_ACCESS_KEY")),
		SecretKey: strings.TrimSpace(os.Getenv("BACKUP_S3_SECRET_KEY")),
		PathStyle: strings.EqualFold(os.Getenv("BACKUP_S3_PATH_STYLE"), "true"),
		Prefix:    strings.TrimSpace(os.Getenv("BACKUP_S3_PREFIX")),
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	if cfg.Prefix == "" {
		cfg.Prefix = "backup/"
	} else if !strings.HasSuffix(cfg.Prefix, "/") {
		cfg.Prefix += "/"
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("BACKUP_S3_BUCKET is not set")
	}
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("BACKUP_S3_ACCESS_KEY / BACKUP_S3_SECRET_KEY not set")
	}
	return &cfg, nil
}

func NewClient(ctx context.Context, c Config) (*Client, error) {
	loaded, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(c.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			c.AccessKey, c.SecretKey, "",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("aws config: %w", err)
	}
	opts := []func(*s3.Options){}
	if c.Endpoint != "" {
		endpoint := c.Endpoint
		opts = append(opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		})
	}
	if c.PathStyle {
		opts = append(opts, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}
	client := s3.NewFromConfig(loaded, opts...)
	return &Client{
		cfg:       c,
		s3:        client,
		presigner: s3.NewPresignClient(client),
		uploader:  manager.NewUploader(client),
	}, nil
}

// ScopedKey returns cfg.Prefix + name. Names containing "/" or ".." are
// rejected to force flat-prefix usage and prevent traversal.
func (c *Client) ScopedKey(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("empty backup name")
	}
	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return "", fmt.Errorf("invalid backup name: %q", name)
	}
	return c.cfg.Prefix + name, nil
}

// Upload streams r into bucket as <prefix>/<name>. Uses the multipart
// uploader so arbitrary-size dumps work without buffering in memory.
func (c *Client) Upload(ctx context.Context, name string, r io.Reader) error {
	key, err := c.ScopedKey(name)
	if err != nil {
		return err
	}
	_, err = c.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(c.cfg.Bucket),
		Key:    aws.String(key),
		Body:   r,
	})
	return err
}

type Object struct {
	Name         string    `json:"name"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
}

// List paginates over all objects under cfg.Prefix. Required for
// retention pruning: with >1000 objects under a single prefix, a
// non-paginating List would silently truncate the result and leave
// old `scheduled-` backups un-deleted.
func (c *Client) List(ctx context.Context) ([]Object, error) {
	objs := make([]Object, 0, 64)
	var continuationToken *string
	for {
		out, err := c.s3.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(c.cfg.Bucket),
			Prefix:            aws.String(c.cfg.Prefix),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return nil, err
		}
		for _, o := range out.Contents {
			obj, ok := convertObject(o, c.cfg.Prefix)
			if ok {
				objs = append(objs, obj)
			}
		}
		if out.IsTruncated == nil || !*out.IsTruncated {
			return objs, nil
		}
		continuationToken = out.NextContinuationToken
	}
}

// convertObject extracts the bare name (stripped of cfg.Prefix), size,
// and last-modified time from an S3 ListObjectsV2 entry. Returns
// ok=false for the prefix itself (zero-name) so callers can skip it.
func convertObject(o s3types.Object, prefix string) (Object, bool) {
	name := strings.TrimPrefix(aws.ToString(o.Key), prefix)
	if name == "" {
		return Object{}, false
	}
	size := int64(0)
	if o.Size != nil {
		size = *o.Size
	}
	lm := time.Time{}
	if o.LastModified != nil {
		lm = *o.LastModified
	}
	return Object{Name: name, Size: size, LastModified: lm}, true
}

func (c *Client) Delete(ctx context.Context, name string) error {
	key, err := c.ScopedKey(name)
	if err != nil {
		return err
	}
	_, err = c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.cfg.Bucket),
		Key:    aws.String(key),
	})
	return err
}

// Get streams the object body. Caller must Close the returned reader.
// Used to feed pg_restore stdin directly from S3 without a temp file.
func (c *Client) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	key, err := c.ScopedKey(name)
	if err != nil {
		return nil, err
	}
	out, err := c.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.cfg.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

// PresignGet returns a time-limited GET URL for downloading an object.
// TTL is capped at 1h to limit exposure window if the URL leaks.
func (c *Client) PresignGet(ctx context.Context, name string, ttl time.Duration) (string, error) {
	key, err := c.ScopedKey(name)
	if err != nil {
		return "", err
	}
	if ttl <= 0 || ttl > time.Hour {
		ttl = time.Hour
	}
	req, err := c.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.cfg.Bucket),
		Key:    aws.String(key),
	}, func(o *s3.PresignOptions) {
		o.Expires = ttl
	})
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

