package s3

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/domain"
)

// HTMLの安全化では実体参照への変換でサイズが増える場合がある（例：&から&amp;）。
// 入力本文は、どちらのオブジェクトも保存する前にユースケースで別途10 MiBに制限する。
const maxObjectBytes = 6 * domain.MaxSourceBytes

type Objects struct {
	client *minio.Client
	bucket string
}

func New(endpoint, bucket, accessKey, secretKey string, secure bool) (*Objects, error) {
	client, err := minio.New(
		endpoint,
		&minio.Options{Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Secure: secure},
	)
	if err != nil {
		return nil, err
	}

	return &Objects{client: client, bucket: bucket}, nil
}

func (s *Objects) Put(ctx context.Context, key, source string) error {
	if len(source) > maxObjectBytes {
		return domain.ErrTooLarge
	}

	_, err := s.client.PutObject(
		ctx,
		s.bucket,
		key,
		strings.NewReader(source),
		int64(len(source)),
		minio.PutObjectOptions{ContentType: "application/octet-stream", DisableMultipart: true},
	)
	if err != nil {
		return fmt.Errorf("%w: object write", domain.ErrUnavailable)
	}

	return nil
}

func (s *Objects) Get(ctx context.Context, key string) (string, error) {
	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return "", domain.ErrUnavailable
	}
	defer func() { _ = object.Close() }()

	content, err := io.ReadAll(io.LimitReader(object, maxObjectBytes+1))
	if err != nil || len(content) > maxObjectBytes {
		return "", domain.ErrUnavailable
	}

	return string(content), nil
}

func (s *Objects) Delete(ctx context.Context, key string) error {
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return domain.ErrUnavailable
	}

	return nil
}

// UnavailableObjects は非公開バケットの接続前でもHTTP契約を安全に維持する。
type UnavailableObjects struct{}

func (UnavailableObjects) Put(context.Context, string, string) error { return domain.ErrUnavailable }
func (UnavailableObjects) Get(context.Context, string) (string, error) {
	return "", domain.ErrUnavailable
}
