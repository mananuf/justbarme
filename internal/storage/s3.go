package storage

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Provider talks to any S3-compatible object store over the standard S3
// API with a custom endpoint -- Cloudflare R2 (the pilot's choice, docs/
// PHASE_PILOT_RELEASE.md §3) and Backblaze B2 both work unchanged, since
// neither is a real AWS account and both accept path-style addressing.
// Objects are uploaded to a public-read bucket and served directly from
// PublicBaseURL -- a business logo isn't sensitive data (it's shown on
// public bill links too), so there is deliberately no proxying of image
// bytes through the API.
type S3Provider struct {
	client        *s3.Client
	bucket        string
	publicBaseURL string
}

// NewS3Provider builds a client pointed at endpoint (e.g. R2's account-
// scoped `https://<account_id>.r2.cloudflarestorage.com`), using
// path-style addressing since most S3-compatible providers other than
// AWS itself expect it. publicBaseURL is where uploaded objects are
// actually reachable from (a bucket's public dev URL, or a custom domain
// in front of it) -- deliberately separate from endpoint, since the
// upload endpoint and the public read path are often different hosts.
func NewS3Provider(ctx context.Context, endpoint, region, accessKeyID, secretAccessKey, bucket, publicBaseURL string) (*S3Provider, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})
	return &S3Provider{
		client:        client,
		bucket:        bucket,
		publicBaseURL: strings.TrimSuffix(publicBaseURL, "/"),
	}, nil
}

func (p *S3Provider) Put(ctx context.Context, key, contentType string, data []byte) (string, error) {
	_, err := p.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(p.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("put object %q: %w", key, err)
	}
	return p.publicBaseURL + "/" + key, nil
}

func (p *S3Provider) URL(key string) string {
	return p.publicBaseURL + "/" + key
}

func (p *S3Provider) Delete(ctx context.Context, key string) error {
	_, err := p.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("delete object %q: %w", key, err)
	}
	return nil
}
