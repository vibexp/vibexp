package storage

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Store is an ObjectStore backed by an S3-compatible bucket: AWS S3, or a
// self-hosted store such as MinIO when Endpoint and PathStyle are set.
type S3Store struct {
	client *s3.Client
	bucket string
}

// S3Config carries the constructor knobs for S3Store so the DI provider can
// hand the config section over without the storage package importing the
// config package.
type S3Config struct {
	// Bucket is the target bucket. Required.
	Bucket string
	// Endpoint overrides the S3 API endpoint (e.g. a MinIO server URL). Empty
	// targets AWS S3 in Region.
	Endpoint string
	// Region is the signing region; required by the SDK even for MinIO (which
	// ignores its value).
	Region string
	// AccessKey / SecretKey are static credentials. Both empty falls back to
	// the AWS SDK default credential chain (env vars, shared config, IAM).
	AccessKey string
	SecretKey string
	// PathStyle forces path-style addressing (endpoint/bucket/key), required
	// by MinIO and most self-hosted S3-compatible stores.
	PathStyle bool
}

// NewS3Store constructs an S3Store. Loading the AWS config never reaches the
// network, but the error is still returned so the provider can degrade
// gracefully (disable attachments) rather than crashing startup.
func NewS3Store(ctx context.Context, cfg S3Config) (*S3Store, error) {
	if cfg.Bucket == "" {
		return nil, errors.New("s3 bucket is empty")
	}
	if cfg.Region == "" {
		return nil, errors.New("s3 region is empty")
	}
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
	}
	if cfg.AccessKey != "" || cfg.SecretKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.PathStyle
	})
	return &S3Store{client: client, bucket: cfg.Bucket}, nil
}

// Upload streams r into the object at key. The caller is responsible for
// bounding r (e.g. with io.LimitReader) — this method copies whatever it reads.
func (s *S3Store) Upload(ctx context.Context, key, contentType string, r io.Reader) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
		Body:        r,
	})
	if err != nil {
		return fmt.Errorf("upload object %q: %w", key, err)
	}
	return nil
}

// Download returns a reader over the object at key. The caller must Close the
// returned body.
func (s *S3Store) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("download object %q: %w", key, err)
	}
	return out.Body, nil
}

// Delete removes the object at key. S3's DeleteObject is idempotent (deleting
// a missing key returns success), so a missing object is not an error —
// matching the GCS delete semantics the attachment service relies on.
func (s *S3Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("delete object %q: %w", key, err)
	}
	return nil
}
