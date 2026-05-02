package storage

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Config holds the options for constructing an S3Provider.
type S3Config struct {
	Bucket string
	Region string
	Prefix string

	// Endpoint overrides the AWS endpoint — use this for S3-compatible services
	// such as MinIO, Wasabi, Backblaze B2, DigitalOcean Spaces, etc.
	// Example: "http://minio:9000" or "https://s3.us-west-000.backblazeb2.com"
	Endpoint string

	// ForcePathStyle disables virtual-hosted-style requests and uses path-style
	// instead (e.g. http://minio:9000/<bucket>/<key>).
	// Required for most S3-compatible services.
	ForcePathStyle bool

	// AccessKeyID and SecretAccessKey supply explicit static credentials.
	// When empty the default credential chain is used (env vars, ~/.aws, IAM).
	AccessKeyID     string
	SecretAccessKey string
}

// S3Provider stores backups in an S3 or S3-compatible bucket.
type S3Provider struct {
	client *s3.Client
	cfg    S3Config
}

// NewS3Provider creates an S3Provider from the supplied S3Config.
func NewS3Provider(ctx context.Context, cfg S3Config) (*S3Provider, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("S3 bucket name must not be empty")
	}

	var optFns []func(*awsconfig.LoadOptions) error

	optFns = append(optFns, awsconfig.WithRegion(cfg.Region))

	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		optFns = append(optFns, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	var s3Opts []func(*s3.Options)

	if cfg.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		})
	}

	if cfg.ForcePathStyle {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}

	client := s3.NewFromConfig(awsCfg, s3Opts...)

	return &S3Provider{client: client, cfg: cfg}, nil
}

// Write uploads a stream to S3 under prefix/key.
func (p *S3Provider) Write(ctx context.Context, key string, r io.Reader) error {
	fullKey := p.fullKey(key)

	_, err := p.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(p.cfg.Bucket),
		Key:    aws.String(fullKey),
		Body:   r,
	})
	if err != nil {
		return fmt.Errorf("s3 PutObject %q: %w", fullKey, err)
	}

	return nil
}

// Read downloads an object from S3 and returns a ReadCloser.
// The caller must close the returned ReadCloser.
func (p *S3Provider) Read(ctx context.Context, key string) (io.ReadCloser, error) {
	fullKey := p.fullKey(key)

	out, err := p.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(p.cfg.Bucket),
		Key:    aws.String(fullKey),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 GetObject %q: %w", fullKey, err)
	}

	return out.Body, nil
}

// Delete removes a single object from S3.
func (p *S3Provider) Delete(ctx context.Context, key string) error {
	fullKey := p.fullKey(key)

	_, err := p.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(p.cfg.Bucket),
		Key:    aws.String(fullKey),
	})
	if err != nil {
		return fmt.Errorf("s3 DeleteObject %q: %w", fullKey, err)
	}

	return nil
}

// List returns all objects whose key starts with prefix/prefix.
func (p *S3Provider) List(ctx context.Context, prefix string) ([]BackupObject, error) {
	fullPrefix := p.fullKey(prefix)

	var objects []BackupObject
	paginator := s3.NewListObjectsV2Paginator(p.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(p.cfg.Bucket),
		Prefix: aws.String(fullPrefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("s3 ListObjectsV2: %w", err)
		}

		for _, obj := range page.Contents {
			if obj.Key == nil {
				continue
			}

			var size int64
			if obj.Size != nil {
				size = *obj.Size
			}

			var lastMod time.Time
			if obj.LastModified != nil {
				lastMod = *obj.LastModified
			}

			// Strip the configured prefix so keys are relative (same as local provider).
			relKey := strings.TrimPrefix(*obj.Key, p.cfg.Prefix)

			objects = append(objects, BackupObject{
				Key:          relKey,
				Size:         size,
				LastModified: lastMod,
				Tags:         storageClassTag(obj.StorageClass),
			})
		}
	}

	return objects, nil
}

// fullKey prepends the configured prefix to a relative key.
func (p *S3Provider) fullKey(key string) string {
	prefix := strings.TrimSuffix(p.cfg.Prefix, "/")
	key = strings.TrimPrefix(key, "/")
	if prefix == "" {
		return key
	}
	return prefix + "/" + key
}

// storageClassTag converts an S3 storage class into a tags map for BackupObject.
func storageClassTag(sc types.ObjectStorageClass) map[string]string {
	if sc == "" {
		return map[string]string{}
	}
	return map[string]string{"storage_class": string(sc)}
}

// Ensure S3Provider implements Provider at compile time.
var _ Provider = (*S3Provider)(nil)
