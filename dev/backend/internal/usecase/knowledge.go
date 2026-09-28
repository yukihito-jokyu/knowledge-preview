package usecase

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/domain"
	nethtml "golang.org/x/net/html"
)

// Sessions はプライマリDBの現在の状態を確認し、JWTだけやキャッシュで判定しない。
// ログアウトは失効をコミットしてから呼び出し元へ成功を返す。
type Sessions interface {
	Active(context.Context, domain.Principal) (bool, error)
}
type KnowledgeRepository interface {
	Get(context.Context, string, string) (domain.Knowledge, error)
	Recent(context.Context, string) ([]domain.Knowledge, error)
	List(context.Context, string, domain.ListQuery) (domain.KnowledgePage, error)
	Draft(context.Context, string, string) (domain.Draft, error)
	CreateDraft(context.Context, domain.Draft) error
	StageObjects(context.Context, []string, func() error) error
	Save(context.Context, domain.Knowledge, int64) (domain.Knowledge, error)
	Commit(context.Context, domain.Draft, domain.Knowledge, string) (string, bool, error)
	Visibility(context.Context, string, string, int64, string, string) (domain.Knowledge, error)
	Grant(context.Context, domain.PreviewGrant) error
	Preview(context.Context, string) (domain.PreviewGrant, error)
	Public(context.Context, string, int64) (domain.Knowledge, error)
	PublicCurrent(context.Context, string) (domain.Knowledge, error)
	PublicRelated(context.Context, string, []string) ([]domain.Knowledge, error)
	Folders(context.Context, string) ([]domain.Folder, error)
	CreateFolder(context.Context, string, string, *string) (domain.Folder, error)
	UpdateFolder(context.Context, string, string, string, *string, bool, int64) (domain.Folder, error)
	DeleteFolder(context.Context, string, string, int64) error
	MoveKnowledge(context.Context, string, string, *string, int64) (domain.Knowledge, error)
}
type SearchTextRepairRepository interface {
	SearchTextBackfill(context.Context, string) ([]domain.Knowledge, error)
	SetSearchText(context.Context, string, string, string) error
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

const searchTextBackfillBatchSize = 100

func NewKnowledgeUseCase(
	repo KnowledgeRepository,
	objects KnowledgeObjects,
	sanitizer HTMLSanitizer,
	sessions Sessions,
) *KnowledgeUseCase {
	return &KnowledgeUseCase{repo: repo, objects: objects, sanitizer: sanitizer, sessions: sessions}
}

// DenySessions は認証を接続しない実行環境やテストでセッションを拒否する。
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

func (u *KnowledgeUseCase) List(
	ctx context.Context,
	p domain.Principal,
	query domain.ListQuery,
) (domain.KnowledgePage, error) {
	if err := u.authorize(ctx, p); err != nil {
		return domain.KnowledgePage{}, err
	}

	query, err := domain.NormalizeListQuery(query)
	if err != nil {
		return domain.KnowledgePage{}, err
	}

	// ponytail: one backfill batch per search request; use a dedicated migration worker if volume grows.
	if query.Query != "" {
		if err := u.repairSearchText(ctx, p.OwnerID); err != nil {
			return domain.KnowledgePage{}, err
		}
	}

	return u.repo.List(ctx, p.OwnerID, query)
}

func (u *KnowledgeUseCase) repairSearchText(ctx context.Context, owner string) error {
	repair, ok := u.repo.(SearchTextRepairRepository)
	if !ok {
		return nil
	}

	items, err := repair.SearchTextBackfill(ctx, owner)
	if err != nil {
		return err
	}

	for _, item := range items {
		source, err := u.objects.Get(ctx, item.SourceKey)
		if err != nil {
			return err
		}

		if err := repair.SetSearchText(ctx, owner, item.ID, searchText(item.Title, item.Tags, source)); err != nil {
			return err
		}
	}

	if len(items) == searchTextBackfillBatchSize {
		remaining, err := repair.SearchTextBackfill(ctx, owner)
		if err != nil {
			return err
		}

		if len(remaining) > 0 {
			return domain.ErrUnavailable
		}
	}

	return nil
}

func searchText(title string, tags []string, source string) string {
	return strings.Join([]string{title, strings.Join(tags, " "), source}, " ")
}

func (u *KnowledgeUseCase) Upload(
	ctx context.Context,
	p domain.Principal,
	filename, source string,
	folderID *string,
) (domain.Draft, error) {
	if err := u.authorize(ctx, p); err != nil {
		return domain.Draft{}, err
	}

	if u.repo == nil || u.objects == nil || (folderID != nil && !domain.ValidID(*folderID)) {
		return domain.Draft{}, domain.ErrBadRequest
	}

	if filename == "" || !utf8.ValidString(filename) || filepath.Base(filename) != filename ||
		strings.ContainsAny(filename, `/\\`) {
		return domain.Draft{}, domain.ErrBadRequest
	}

	ext := strings.ToLower(filepath.Ext(filename))

	format := map[string]string{".md": "markdown", ".html": "html"}[ext]
	if format == "" {
		return domain.Draft{}, &domain.ValidationError{
			Detail: domain.FieldError{Field: "file", Reason: "unsupported_format"},
		}
	}

	title := strings.TrimSpace(strings.TrimSuffix(filename, filepath.Ext(filename)))
	if utf8.RuneCountInString(title) < 1 || utf8.RuneCountInString(title) > 200 {
		return domain.Draft{}, &domain.ValidationError{
			Detail: domain.FieldError{Field: "file", Reason: "invalid_title"},
		}
	}

	k := domain.Knowledge{
		OwnerID:        p.OwnerID,
		Title:          title,
		Format:         format,
		Tags:           []string{},
		LearningStatus: "unlearned",
		Version:        1,
		Folder:         folderFromID(folderID),
	}

	k, _, err := domain.ApplySource(k, source)
	if err != nil {
		return domain.Draft{}, err
	}

	if format == "html" {
		if u.sanitizer == nil {
			return domain.Draft{}, domain.ErrUnavailable
		}

		if _, _, err := u.sanitizer.Sanitize(source); err != nil {
			return domain.Draft{}, err
		}
	}

	draftID := newID()

	key := "knowledge/" + randomToken()
	if err := u.repo.StageObjects(ctx, []string{key}, func() error {
		return u.objects.Put(ctx, key, source)
	}); err != nil {
		return domain.Draft{}, err
	}

	draft := domain.Draft{Knowledge: k, DraftID: draftID}

	draft.SourceKey = key
	if err := u.repo.CreateDraft(ctx, draft); err != nil {
		return domain.Draft{}, err
	}

	return draft, nil
}

func folderFromID(id *string) *domain.Folder {
	if id == nil {
		return nil
	}

	return &domain.Folder{ID: *id}
}

func (u *KnowledgeUseCase) Folders(ctx context.Context, p domain.Principal) ([]domain.Folder, error) {
	if err := u.authorize(ctx, p); err != nil {
		return nil, err
	}

	return u.repo.Folders(ctx, p.OwnerID)
}

func (u *KnowledgeUseCase) CreateFolder(
	ctx context.Context,
	p domain.Principal,
	name string,
	parentID *string,
) (domain.Folder, error) {
	if err := u.authorize(ctx, p); err != nil {
		return domain.Folder{}, err
	}

	name, err := domain.ValidateFolderName(name)
	if err != nil {
		return domain.Folder{}, err
	}

	if parentID != nil && !domain.ValidID(*parentID) {
		return domain.Folder{}, domain.ErrBadRequest
	}

	return u.repo.CreateFolder(ctx, p.OwnerID, name, parentID)
}

func (u *KnowledgeUseCase) UpdateFolder(
	ctx context.Context,
	p domain.Principal,
	id, name string,
	parentID *string,
	parentSpecified bool,
	version int64,
) (domain.Folder, error) {
	if err := u.authorize(ctx, p); err != nil {
		return domain.Folder{}, err
	}

	if !domain.ValidID(id) || version < 1 || version > domain.MaxVersion {
		return domain.Folder{}, domain.ErrBadRequest
	}

	name, err := domain.ValidateFolderName(name)
	if err != nil {
		return domain.Folder{}, err
	}

	if parentSpecified && parentID != nil && !domain.ValidID(*parentID) {
		return domain.Folder{}, domain.ErrBadRequest
	}

	return u.repo.UpdateFolder(ctx, p.OwnerID, id, name, parentID, parentSpecified, version)
}

func (u *KnowledgeUseCase) DeleteFolder(ctx context.Context, p domain.Principal, id string, version int64) error {
	if err := u.authorize(ctx, p); err != nil {
		return err
	}

	if !domain.ValidID(id) || version < 1 || version > domain.MaxVersion {
		return domain.ErrBadRequest
	}

	return u.repo.DeleteFolder(ctx, p.OwnerID, id, version)
}

func (u *KnowledgeUseCase) MoveKnowledge(
	ctx context.Context,
	p domain.Principal,
	id string,
	folderID *string,
	version int64,
) (domain.Knowledge, error) {
	if err := u.authorize(ctx, p); err != nil {
		return domain.Knowledge{}, err
	}

	if !domain.ValidID(id) || version < 1 || version > domain.MaxVersion {
		return domain.Knowledge{}, domain.ErrBadRequest
	}

	return u.repo.MoveKnowledge(ctx, p.OwnerID, id, folderID, version)
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
	k.SearchText = searchText(k.Title, k.Tags, source)
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

func (u *KnowledgeUseCase) Public(
	ctx context.Context,
	id string,
) (domain.Knowledge, string, string, []domain.PublicRelated, error) {
	if !domain.ValidPublicID(id) {
		return domain.Knowledge{}, "", "", nil, domain.ErrNotFound
	}

	if u.repo == nil {
		return domain.Knowledge{}, "", "", nil, domain.ErrUnavailable
	}

	k, err := u.repo.PublicCurrent(ctx, id)
	if err != nil {
		return domain.Knowledge{}, "", "", nil, err
	}

	source := ""
	summary := ""

	switch k.Format {
	case "markdown":
		if u.objects == nil {
			return domain.Knowledge{}, "", "", nil, domain.ErrUnavailable
		}

		source, err = u.objects.Get(ctx, k.SourceKey)
		if err != nil {
			return domain.Knowledge{}, "", "", nil, err
		}

		k, source, err = domain.ApplySource(k, source)
		if err != nil {
			return domain.Knowledge{}, "", "", nil, domain.ErrUnavailable
		}

		summary = markdownSummary(source)
	case "html":
		if u.objects == nil {
			return domain.Knowledge{}, "", "", nil, domain.ErrUnavailable
		}

		if _, err = u.objects.Get(ctx, k.HTMLKey); err != nil {
			return domain.Knowledge{}, "", "", nil, err
		}
	}

	var related []domain.PublicRelated

	if len(k.Tags) > 0 {
		candidates, relatedErr := u.repo.PublicRelated(ctx, id, k.Tags)
		if relatedErr == nil {
			related = make([]domain.PublicRelated, 0, len(candidates))
			for _, candidate := range candidates {
				if candidate.PublicID == nil {
					related = nil
					break
				}

				summary := ""

				if candidate.Format == "markdown" {
					if u.objects == nil {
						related = nil
						break
					}

					candidateSource, getErr := u.objects.Get(ctx, candidate.SourceKey)
					if getErr != nil {
						related = nil
						break
					}

					candidate, candidateSource, getErr = domain.ApplySource(candidate, candidateSource)
					if getErr != nil {
						related = nil
						break
					}

					summary = markdownSummary(candidateSource)
				}

				related = append(related, domain.PublicRelated{
					PublicID: *candidate.PublicID,
					Title:    candidate.Title,
					Format:   candidate.Format,
					Summary:  summary,
				})
			}
		}
	}

	return k, source, summary, related, nil
}

var markdownParser = goldmark.New(goldmark.WithExtensions(extension.GFM))

func markdownSummary(body string) string {
	body = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(body)
	source := []byte(body)
	document := markdownParser.Parser().Parse(text.NewReader(source))

	for child := document.FirstChild(); child != nil; child = child.NextSibling() {
		paragraph, ok := child.(*ast.Paragraph)
		if !ok {
			continue
		}

		summary := markdownParagraphText(paragraph, source)
		if summary == "" {
			continue
		}

		return summary
	}

	return ""
}

func markdownParagraphText(paragraph ast.Node, source []byte) string {
	normalizeMarkdownTextNodes(paragraph, source)

	var rendered bytes.Buffer
	if err := markdownParser.Renderer().Render(&rendered, source, paragraph); err != nil {
		return ""
	}

	document, err := nethtml.Parse(strings.NewReader(rendered.String()))
	if err != nil {
		return ""
	}

	var (
		plain strings.Builder
		visit func(*nethtml.Node)
	)

	visit = func(node *nethtml.Node) {
		if node.Type == nethtml.TextNode {
			plain.WriteString(node.Data)
		}

		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(document)

	return strings.Join(strings.Fields(plain.String()), " ")
}

func normalizeMarkdownTextNodes(paragraph ast.Node, source []byte) {
	var (
		texts        []*ast.Text
		stringsNodes []*ast.String
	)

	_ = ast.Walk(paragraph, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		if node.Kind() == ast.KindImage {
			return ast.WalkSkipChildren, nil
		}

		switch node := node.(type) {
		case *ast.Text:
			if !node.IsRaw() {
				texts = append(texts, node)
			}
		case *ast.String:
			if !node.IsRaw() && !node.IsCode() {
				stringsNodes = append(stringsNodes, node)
			}
		}

		return ast.WalkContinue, nil
	})

	for _, node := range texts {
		normalizeMarkdownTextNode(node.Parent(), node, source)
	}

	for _, node := range stringsNodes {
		normalized := normalizeMarkdownNumericText(node.Value)
		if normalized != nil {
			node.Value = normalized
		}
	}
}

func normalizeMarkdownTextNode(parent ast.Node, node *ast.Text, source []byte) {
	segment := node.Segment
	raw := source[segment.Start:segment.Stop]

	replacements := markdownInvalidNumericReferences(raw)
	if len(replacements) == 0 {
		return
	}

	replacementNodes := make([]ast.Node, 0, len(replacements)*2+3)
	if segment.Padding > 0 {
		replacementNodes = append(replacementNodes, ast.NewString(bytes.Repeat([]byte(" "), segment.Padding)))
	}

	position := 0
	for _, replacement := range replacements {
		if position < replacement.start {
			replacementNodes = append(replacementNodes, ast.NewTextSegment(markdownTextSegment(
				segment.Start+position,
				segment.Start+replacement.start,
				false,
			)))
		}

		replacementNodes = append(replacementNodes, ast.NewString([]byte("�")))
		position = replacement.end
	}

	if position < len(raw) {
		replacementNodes = append(replacementNodes, ast.NewTextSegment(markdownTextSegment(
			segment.Start+position,
			segment.Stop,
			segment.ForceNewline,
		)))
	} else if node.SoftLineBreak() || node.HardLineBreak() {
		replacementNodes = append(replacementNodes, ast.NewTextSegment(markdownTextSegment(
			segment.Stop,
			segment.Stop,
			segment.ForceNewline,
		)))
	}

	for index := len(replacementNodes) - 1; index >= 0; index-- {
		if replacementText, ok := replacementNodes[index].(*ast.Text); ok {
			replacementText.SetSoftLineBreak(node.SoftLineBreak())
			replacementText.SetHardLineBreak(node.HardLineBreak())

			break
		}
	}

	parent.ReplaceChild(parent, node, replacementNodes[0])

	previous := replacementNodes[0]
	for _, replacement := range replacementNodes[1:] {
		parent.InsertAfter(parent, previous, replacement)
		previous = replacement
	}
}

type markdownNumericReplacement struct {
	start int
	end   int
}

func markdownInvalidNumericReferences(source []byte) []markdownNumericReplacement {
	var replacements []markdownNumericReplacement

	for index := 0; index < len(source); index++ {
		if source[index] != '&' || markdownTextEscaped(source, index) {
			continue
		}

		end, invalid := invalidMarkdownNumericReference(source, index)
		if invalid {
			replacements = append(replacements, markdownNumericReplacement{start: index, end: end})
			index = end - 1
		}
	}

	return replacements
}

func invalidMarkdownNumericReference(source []byte, start int) (int, bool) {
	if start+2 >= len(source) || source[start] != '&' || source[start+1] != '#' {
		return start, false
	}

	valueStart := start + 2
	base := 10
	maxDigits := 7

	if source[valueStart] == 'x' || source[valueStart] == 'X' {
		valueStart++
		base = 16
		maxDigits = 6
	}

	valueEnd := valueStart
	for valueEnd < len(source) && markdownNumericDigit(source[valueEnd], base) {
		valueEnd++
	}

	if valueEnd == valueStart || valueEnd-valueStart > maxDigits || valueEnd >= len(source) || source[valueEnd] != ';' {
		return start, false
	}

	value, err := strconv.ParseUint(string(source[valueStart:valueEnd]), base, 32)
	if err != nil || markdownInvalidCodePoint(value) {
		return valueEnd + 1, true
	}

	return start, false
}

func normalizeMarkdownNumericText(source []byte) []byte {
	replacements := markdownInvalidNumericReferences(source)
	if len(replacements) == 0 {
		return nil
	}

	normalized := make([]byte, 0, len(source))

	position := 0
	for _, replacement := range replacements {
		normalized = append(normalized, source[position:replacement.start]...)
		normalized = append(normalized, []byte("�")...)
		position = replacement.end
	}

	return append(normalized, source[position:]...)
}

func markdownTextEscaped(source []byte, index int) bool {
	backslashes := 0
	for index--; index >= 0 && source[index] == '\\'; index-- {
		backslashes++
	}

	return backslashes%2 == 1
}

func markdownNumericDigit(c byte, base int) bool {
	if c >= '0' && c <= '9' {
		return true
	}

	return base == 16 && ((c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F'))
}

func markdownInvalidCodePoint(code uint64) bool {
	return code < 9 || code == 11 || code > 13 && code < 32 ||
		code > 126 && code < 160 || code > 55295 && code < 57344 ||
		code > 64975 && code < 65008 || code&65535 == 65535 ||
		code&65535 == 65534 || code > 1114111
}

func markdownTextSegment(start, stop int, forceNewline bool) text.Segment {
	segment := text.NewSegment(start, stop)
	segment.ForceNewline = forceNewline

	return segment
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
