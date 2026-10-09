package app

import (
	"context"
	"errors"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"net/url"
	"strings"
	"time"
)

type ObjectInfo struct {
	Size        int64
	ContentType string
}
type ObjectStore interface {
	PresignPut(context.Context, string, time.Duration) (string, error)
	PresignGet(context.Context, string, time.Duration) (string, error)
	Stat(context.Context, string) (ObjectInfo, error)
	Delete(context.Context, string) error
}
type S3ObjectStore struct {
	client *minio.Client
	bucket string
}

func NewS3ObjectStore(endpoint, accessKey, secretKey, bucket string) (*S3ObjectStore, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return nil, errors.New("invalid S3 endpoint")
	}
	if accessKey == "" || secretKey == "" || bucket == "" {
		return nil, errors.New("S3 credentials and bucket are required")
	}
	client, err := minio.New(parsed.Host, &minio.Options{Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Secure: parsed.Scheme == "https", Region: ""})
	if err != nil {
		return nil, err
	}
	return &S3ObjectStore{client: client, bucket: bucket}, nil
}
func (s *S3ObjectStore) PresignPut(ctx context.Context, key string, expiry time.Duration) (string, error) {
	value, err := s.client.PresignedPutObject(ctx, s.bucket, key, expiry)
	if err != nil {
		return "", err
	}
	return value.String(), nil
}
func (s *S3ObjectStore) PresignGet(ctx context.Context, key string, expiry time.Duration) (string, error) {
	value, err := s.client.PresignedGetObject(ctx, s.bucket, key, expiry, nil)
	if err != nil {
		return "", err
	}
	return value.String(), nil
}
func (s *S3ObjectStore) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	value, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	return ObjectInfo{Size: value.Size, ContentType: value.ContentType}, err
}
func (s *S3ObjectStore) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}
