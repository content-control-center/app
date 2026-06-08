package storage

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/ogen-app/ogen/src/config"
)

// Storage is the interface for object storage backends.
type Storage interface {
	// Upload writes r to the bucket under key and returns the public URL.
	Upload(ctx context.Context, key string, r io.Reader, size int64, contentType string) (string, error)
	// Copy duplicates the object at srcKey to dstKey within the same
	// bucket. Used by post cloning (CON-59) to give a clone its own
	// independent copies of its source's attachments, so deleting
	// either post never removes the other's blobs.
	Copy(ctx context.Context, srcKey, dstKey string) error
	// Delete removes the object at key. Returns nil when the object does not exist.
	Delete(ctx context.Context, key string) error
	// PublicURL returns the public URL for an object at key.
	PublicURL(key string) string
	// PresignedGetURL returns a short-lived signed GET URL for the object
	// at key. Used for serving private images to the browser and for
	// handing image bytes to Zernio at publish time (CON-73).
	PresignedGetURL(ctx context.Context, key string, ttl time.Duration) (string, error)
}

type s3Storage struct {
	client    *s3.Client
	presign   *s3.PresignClient
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

	return &s3Storage{
		client:    client,
		presign:   s3.NewPresignClient(client),
		bucket:    cfg.StorageBucket,
		publicURL: publicURL,
	}, nil
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
	return s.PublicURL(key), nil
}

func (s *s3Storage) Copy(ctx context.Context, srcKey, dstKey string) error {
	_, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(s.bucket),
		CopySource: aws.String(s.bucket + "/" + srcKey),
		Key:        aws.String(dstKey),
	})
	if err != nil {
		return fmt.Errorf("storage: copy %s -> %s: %w", srcKey, dstKey, err)
	}
	return nil
}

func (s *s3Storage) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("storage: delete %s: %w", key, err)
	}
	return nil
}

func (s *s3Storage) PublicURL(key string) string {
	return s.publicURL + "/" + key
}

func (s *s3Storage) PresignedGetURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	req, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("storage: presign get %s: %w", key, err)
	}
	return req.URL, nil
}
