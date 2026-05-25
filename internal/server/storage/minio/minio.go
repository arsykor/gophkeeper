// Package minio provides object storage for large binary secrets using MinIO.
package minio

import (
	"context"
	"fmt"
	"io"

	miniogo "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Client wraps a MinIO client and manages a single bucket.
type Client struct {
	mc     *miniogo.Client
	bucket string
}

// New creates a MinIO client and ensures the bucket exists.
func New(endpoint, accessKey, secretKey, bucket string, useSSL bool) (*Client, error) {
	mc, err := miniogo.New(endpoint, &miniogo.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, err
	}
	c := &Client{mc: mc, bucket: bucket}
	if err := c.ensureBucket(context.Background()); err != nil {
		return nil, err
	}
	return c, nil
}

// ObjectKey returns the MinIO object key for a given user + secret IDs.
func ObjectKey(userID, secretID string) string {
	return fmt.Sprintf("%s/%s", userID, secretID)
}

// Upload streams r into the object identified by key. size is -1 when unknown.
func (c *Client) Upload(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err := c.mc.PutObject(ctx, c.bucket, key, r, size, miniogo.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}

// Download returns a reader for the object identified by key.
// The caller is responsible for closing the reader.
func (c *Client) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := c.mc.GetObject(ctx, c.bucket, key, miniogo.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// Delete removes the object identified by key.
func (c *Client) Delete(ctx context.Context, key string) error {
	return c.mc.RemoveObject(ctx, c.bucket, key, miniogo.RemoveObjectOptions{})
}

func (c *Client) ensureBucket(ctx context.Context) error {
	exists, err := c.mc.BucketExists(ctx, c.bucket)
	if err != nil {
		return err
	}
	if !exists {
		return c.mc.MakeBucket(ctx, c.bucket, miniogo.MakeBucketOptions{})
	}
	return nil
}
