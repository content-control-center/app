package storage

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/content-control-center/app/src/config"
)

// Storage is the interface for object storage backends.
type Storage interface {
	// Upload writes r to the bucket under key and returns the public URL.
	Upload(ctx context.Context, key string, r io.Reader, size int64, contentType string) (string, error)
}

type s3Storage struct {
	client    *s3.Client
	bucket    string
	publicURL string // base URL, no trailing slash
}

// New returns an S3-compatible Storage client configured from cfg.
// Returns nil, nil when StorageEndpoint is empty (uploads disabled).
func New(cfg *config.Config) (Storage, error) {
	if cfg.StorageEndpoint == "" {
		return nil, nil
	}

	creds := credentials.NewStaticCredentialsProvider(cfg.StorageAccessKey, cfg.StorageSecretKey, "")

	client := s3.New(s3.Options{
		Region:      cfg.StorageRegion,
		Credentials: creds,
		BaseEndpoint: aws.String(cfg.StorageEndpoint),
		// Required for path-style access used by R2 and DO Spaces.
		UsePathStyle: true,
	})

	publicURL := strings.TrimRight(cfg.StoragePublicURL, "/")

	return &s3Storage{client: client, bucket: cfg.StorageBucket, publicURL: publicURL}, nil
}

func (s *s3Storage) Upload(ctx context.Context, key string, r io.Reader, size int64, contentType string) (string, error) {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          r,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("storage: upload %s: %w", key, err)
	}
	return s.publicURL + "/" + key, nil
}
