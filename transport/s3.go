package transport

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Client struct {
	endpoint  string
	accessKey string
	secretKey string
	bucket    string
	prefix    string
	client    *minio.Client
}

func NewS3(cfg map[string]string) (*S3Client, error) {
	return &S3Client{
		endpoint:  cfg["endpoint"],
		accessKey: cfg["access_key"],
		secretKey: cfg["secret_key"],
		bucket:    cfg["bucket"],
		prefix:    cfg["prefix"],
	}, nil
}

func (s *S3Client) Name() string { return "s3" }

func (s *S3Client) connect() (*minio.Client, error) {
	if s.client != nil {
		return s.client, nil
	}
	client, err := minio.New(s.endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(s.accessKey, s.secretKey, ""),
		Secure: false,
	})
	if err != nil {
		return nil, err
	}
	s.client = client
	return client, nil
}

func (s *S3Client) key(remotePath string) string {
	return strings.TrimLeft(filepath.Join(s.prefix, remotePath), "/")
}

func (s *S3Client) List(remotePath string) ([]FileInfo, error) {
	c, err := s.connect()
	if err != nil {
		return nil, err
	}
	prefix := s.key(remotePath)
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	var result []FileInfo
	ctx := context.Background()
	for obj := range c.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}) {
		if obj.Err != nil {
			return nil, obj.Err
		}
		result = append(result, FileInfo{
			Name:  filepath.Base(obj.Key),
			Path:  strings.TrimPrefix(obj.Key, s.prefix+"/"),
			Size:  obj.Size,
			IsDir: strings.HasSuffix(obj.Key, "/"),
		})
	}
	return result, nil
}

func (s *S3Client) Push(localPath, remotePath string) error {
	ctx := context.Background()
	c, err := s.connect()
	if err != nil {
		return err
	}
	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = c.PutObject(ctx, s.bucket, s.key(remotePath), file, -1, minio.PutObjectOptions{})
	return err
}

func (s *S3Client) Pull(remotePath, localPath string) error {
	ctx := context.Background()
	c, err := s.connect()
	if err != nil {
		return err
	}
	obj, err := c.GetObject(ctx, s.bucket, s.key(remotePath), minio.GetObjectOptions{})
	if err != nil {
		return err
	}
	defer obj.Close()

	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return err
	}
	out, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, obj)
	return err
}

func (s *S3Client) Delete(remotePath string) error {
	ctx := context.Background()
	c, err := s.connect()
	if err != nil {
		return err
	}
	return c.RemoveObject(ctx, s.bucket, s.key(remotePath), minio.RemoveObjectOptions{})
}

func (s *S3Client) Test() error {
	c, err := s.connect()
	if err != nil {
		return err
	}
	_, err = c.ListBuckets(context.Background())
	_ = bytes.NewReader // keep import
	return err
}
