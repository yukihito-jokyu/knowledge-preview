package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/domain"
)

// Sessions はプライマリDBの現在の状態を確認し、JWTだけやキャッシュで判定しない。
// ログアウトは失効をコミットしてから呼び出し元へ成功を返す。
type Sessions interface {
	Active(context.Context, domain.Principal) (bool, error)
}
type KnowledgeRepository interface {
	Get(context.Context, string, string) (domain.Knowledge, error)
	Recent(context.Context, string) ([]domain.Knowledge, error)
	Draft(context.Context, string, string) (domain.Draft, error)
	StageObjects(context.Context, []string, func() error) error
	Save(context.Context, domain.Knowledge, int64) (domain.Knowledge, error)
	Commit(context.Context, domain.Draft, domain.Knowledge, string) (string, bool, error)
	Visibility(context.Context, string, string, int64, string, string) (domain.Knowledge, error)
	Grant(context.Context, domain.PreviewGrant) error
	Preview(context.Context, string) (domain.PreviewGrant, error)
	Public(context.Context, string, int64) (domain.Knowledge, error)
}
type KnowledgeObjects interface {
	Put(context.Context, string, string) error
	Get(context.Context, string) (string, error)
}
type HTMLSanitizer interface {
	Sanitize(string) (string, bool, error)
}
type KnowledgeUseCase struct {
	repo      KnowledgeRepository
	objects   KnowledgeObjects
	sanitizer HTMLSanitizer
	sessions  Sessions
}

func NewKnowledgeUseCase(
	repo KnowledgeRepository,
	objects KnowledgeObjects,
	sanitizer HTMLSanitizer,
	sessions Sessions,
) *KnowledgeUseCase {
	return &KnowledgeUseCase{repo: repo, objects: objects, sanitizer: sanitizer, sessions: sessions}
}

// DenySessions は#19で認証済みセッションの保存先を接続するまで、セッションを拒否する。
type DenySessions struct{}

func (DenySessions) Active(context.Context, domain.Principal) (bool, error) { return false, nil }
func (u *KnowledgeUseCase) authorize(ctx context.Context, p domain.Principal) error {
	if p.OwnerID == "" || p.SessionID == "" || u.sessions == nil {
		return domain.ErrUnauthenticated
	}

	active, err := u.sessions.Active(ctx, p)
	if err != nil {
		return domain.ErrUnavailable
	}

	if !active {
		return domain.ErrUnauthenticated
	}

	return nil
}

func (u *KnowledgeUseCase) Get(ctx context.Context, p domain.Principal, id string) (domain.Knowledge, string, error) {
	if err := u.authorize(ctx, p); err != nil {
		return domain.Knowledge{}, "", err
	}

	k, err := u.repo.Get(ctx, p.OwnerID, id)
	if err != nil {
		return k, "", err
	}

	source, err := u.objects.Get(ctx, k.SourceKey)

	return k, source, err
}

func (u *KnowledgeUseCase) Recent(ctx context.Context, p domain.Principal) ([]domain.Knowledge, error) {
	if err := u.authorize(ctx, p); err != nil {
		return nil, err
	}

	return u.repo.Recent(ctx, p.OwnerID)
}

func (u *KnowledgeUseCase) Draft(ctx context.Context, p domain.Principal, id string) (domain.Draft, string, error) {
	if err := u.authorize(ctx, p); err != nil {
		return domain.Draft{}, "", err
	}

	d, err := u.repo.Draft(ctx, p.OwnerID, id)
	if err != nil || d.CommittedID != "" {
		return d, "", err
	}

	source, err := u.objects.Get(ctx, d.SourceKey)

	return d, source, err
}

func (u *KnowledgeUseCase) prepare(
	ctx context.Context,
	k domain.Knowledge,
	source string,
) (domain.Knowledge, bool, error) {
	k, _, err := domain.ApplySource(k, source)
	if err != nil {
		return k, false, err
	}

	var (
		clean   string
		warning bool
	)
	if k.Format == "html" {
		clean, warning, err = u.sanitizer.Sanitize(source)
		if err != nil {
			return k, false, err
		}
	}

	k.HTMLSanitized = warning
	k.SourceKey = "knowledge/" + randomToken()
	keys := []string{k.SourceKey}

	k.HTMLKey = ""
	if k.Format == "html" {
		k.HTMLKey = "knowledge/" + randomToken()
		keys = append(keys, k.HTMLKey)
	}
	// Putの前に記録する。失敗・結果不明の書き込みは保留し、参照状況を確認する回収処理に任せる。
	err = u.repo.StageObjects(ctx, keys, func() error {
		if err := u.objects.Put(ctx, k.SourceKey, source); err != nil {
			return err
		}

		if k.HTMLKey != "" {
			return u.objects.Put(ctx, k.HTMLKey, clean)
		}

		return nil
	})
	if err != nil {
		return k, false, err
	}

	return k, warning, nil
}

func (u *KnowledgeUseCase) Save(
	ctx context.Context,
	p domain.Principal,
	id string,
	version int64,
	source string,
) (domain.Knowledge, bool, error) {
	if err := u.authorize(ctx, p); err != nil {
		return domain.Knowledge{}, false, err
	}

	k, err := u.repo.Get(ctx, p.OwnerID, id)
	if err != nil {
		return k, false, err
	}

	if k.Version != version || version >= domain.MaxVersion {
		return k, false, domain.ErrConflict
	}

	k, warning, err := u.prepare(ctx, k, source)
	if err != nil {
		return k, false, err
	}

	k, err = u.repo.Save(ctx, k, version)

	return k, warning, err
}

func (u *KnowledgeUseCase) Commit(
	ctx context.Context,
	p domain.Principal,
	id string,
	version int64,
	source string,
) (string, bool, error) {
	if err := u.authorize(ctx, p); err != nil {
		return "", false, err
	}

	d, err := u.repo.Draft(ctx, p.OwnerID, id)
	if err != nil {
		return "", false, err
	}

	if version != d.Version {
		return "", false, domain.ErrConflict
	}
	// 変更不可のdraftの形式・フォルダー・メタデータと要求バージョンは、draft行から取得する。
	hash := sha256.Sum256([]byte(source))

	digest := hex.EncodeToString(hash[:])
	if d.CommittedID != "" {
		if d.CommitHash == digest {
			return d.CommittedID, false, nil
		}

		return "", false, domain.ErrConflict
	}

	k, _, err := u.prepare(ctx, d.Knowledge, source)
	if err != nil {
		return "", false, err
	}

	k.ID = newID()
	k.Visibility = "private"
	k.PublicID = nil

	return u.repo.Commit(ctx, d, k, digest)
}

func (u *KnowledgeUseCase) Visibility(
	ctx context.Context,
	p domain.Principal,
	id string,
	version int64,
	visibility string,
) (domain.Knowledge, string, error) {
	if err := u.authorize(ctx, p); err != nil {
		return domain.Knowledge{}, "", err
	}

	if visibility != "private" && visibility != "unlisted" {
		return domain.Knowledge{}, "", domain.ErrBadRequest
	}

	current, err := u.repo.Get(ctx, p.OwnerID, id)
	if err != nil {
		return current, "", err
	}

	source, err := u.objects.Get(ctx, current.SourceKey)
	if err != nil {
		return current, "", err
	}

	k, err := u.repo.Visibility(ctx, p.OwnerID, id, version, visibility, randomToken())

	return k, source, err
}

func (u *KnowledgeUseCase) PreviewTicket(
	ctx context.Context,
	p domain.Principal,
	id string,
	version int64,
) (string, error) {
	if err := u.authorize(ctx, p); err != nil {
		return "", err
	}

	k, err := u.repo.Get(ctx, p.OwnerID, id)
	if err != nil {
		return "", err
	}

	if k.Version != version {
		return "", domain.ErrConflict
	}

	if k.Format != "html" {
		return "", domain.ErrValidation
	}

	token := randomToken()
	hash := sha256.Sum256([]byte(token))
	err = u.repo.Grant(
		ctx,
		domain.PreviewGrant{
			Hash:        hex.EncodeToString(hash[:]),
			OwnerID:     p.OwnerID,
			SessionID:   p.SessionID,
			KnowledgeID: id,
			Version:     version,
		},
	)

	return token, err
}

func (u *KnowledgeUseCase) PrivateHTML(ctx context.Context, token string) (string, error) {
	if len(token) != 43 || u.sessions == nil {
		return "", domain.ErrNotFound
	}

	hash := sha256.Sum256([]byte(token))

	grant, err := u.repo.Preview(ctx, hex.EncodeToString(hash[:]))
	if err != nil {
		return "", err
	}
	// Cookieのないリクエストも含め、毎回URL発行元のセッションを再確認する。
	active, err := u.sessions.Active(ctx, domain.Principal{OwnerID: grant.OwnerID, SessionID: grant.SessionID})
	if err != nil {
		return "", domain.ErrUnavailable
	}

	if !active {
		return "", domain.ErrNotFound
	}

	k, err := u.repo.Get(ctx, grant.OwnerID, grant.KnowledgeID)
	if err != nil {
		return "", err
	}

	if k.Version != grant.Version || k.Format != "html" {
		return "", domain.ErrNotFound
	}

	return u.objects.Get(ctx, k.HTMLKey)
}

func (u *KnowledgeUseCase) PublicHTML(ctx context.Context, id string, version int64) (string, error) {
	k, err := u.repo.Public(ctx, id, version)
	if err != nil {
		return "", err
	}

	if k.Format != "html" {
		return "", domain.ErrNotFound
	}

	return u.objects.Get(ctx, k.HTMLKey)
}

func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)

	return base64.RawURLEncoding.EncodeToString(b)
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
