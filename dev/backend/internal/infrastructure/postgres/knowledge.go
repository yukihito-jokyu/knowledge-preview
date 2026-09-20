package postgres

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/domain"
)

type KnowledgeRepository struct{ pool *pgxpool.Pool }

func NewKnowledgeRepository(pool *pgxpool.Pool) *KnowledgeRepository {
	return &KnowledgeRepository{pool: pool}
}

const knowledgeColumns = `k.id::text,k.owner_id,k.title,k.format,k.tags,k.learning_status,f.id::text,f.name,f.parent_id::text,f.version,k.version,k.updated_at,k.visibility,k.public_id,k.source_key,COALESCE(k.html_key,''),k.html_sanitized`

func scanKnowledge(row pgx.Row) (domain.Knowledge, error) {
	var (
		k                                    domain.Knowledge
		folderID, folderName, folderParentID *string
		folderVersion                        *int64
	)

	err := row.Scan(
		&k.ID,
		&k.OwnerID,
		&k.Title,
		&k.Format,
		&k.Tags,
		&k.LearningStatus,
		&folderID,
		&folderName,
		&folderParentID,
		&folderVersion,
		&k.Version,
		&k.UpdatedAt,
		&k.Visibility,
		&k.PublicID,
		&k.SourceKey,
		&k.HTMLKey,
		&k.HTMLSanitized,
	)
	if folderID != nil && folderName != nil && folderVersion != nil {
		k.Folder = &domain.Folder{ID: *folderID, Name: *folderName, ParentID: folderParentID, Version: *folderVersion}
	}

	return k, dbError(err)
}

func dbError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return domain.ErrConflict
		case "23503":
			return domain.ErrNotFound
		}
	}

	return fmt.Errorf("%w: %w", domain.ErrUnavailable, err)
}

func (r *KnowledgeRepository) Get(ctx context.Context, owner, id string) (domain.Knowledge, error) {
	return scanKnowledge(
		r.pool.QueryRow(
			ctx,
			`SELECT `+knowledgeColumns+` FROM knowledge k LEFT JOIN knowledge_folders f ON f.id=k.folder_id WHERE k.id=$1 AND k.owner_id=$2`,
			id,
			owner,
		),
	)
}

func (r *KnowledgeRepository) Recent(ctx context.Context, owner string) ([]domain.Knowledge, error) {
	rows, err := r.pool.Query(
		ctx,
		`SELECT `+knowledgeColumns+` FROM knowledge k LEFT JOIN knowledge_folders f ON f.id=k.folder_id WHERE k.owner_id=$1 ORDER BY k.updated_at DESC,k.id ASC LIMIT 10`,
		owner,
	)
	if err != nil {
		return nil, dbError(err)
	}
	defer rows.Close()

	result := []domain.Knowledge{}

	for rows.Next() {
		k, err := scanKnowledge(rows)
		if err != nil {
			return nil, err
		}

		result = append(result, k)
	}

	return result, dbError(rows.Err())
}

func (r *KnowledgeRepository) List(
	ctx context.Context,
	owner string,
	q domain.ListQuery,
) (domain.KnowledgePage, error) {
	args := []any{owner}
	where := []string{"k.owner_id=$1"}

	arg := 2
	if q.Query != "" {
		condition, searchArgs := fullTextSearchCondition(arg, q.Query)
		where = append(where, condition)
		args = append(args, searchArgs...)
		arg += len(searchArgs)
	}

	if len(q.Tags) > 0 {
		where = append(where, "k.tags @> $"+fmt.Sprint(arg))
		args = append(args, q.Tags)
		arg++
	}

	if q.FolderID != nil {
		if *q.FolderID == "" {
			where = append(where, "k.folder_id IS NULL")
		} else {
			where = append(where, "k.folder_id=$"+fmt.Sprint(arg))
			args = append(args, *q.FolderID)
			arg++
		}
	}

	whereSQL := strings.Join(where, " AND ")

	var page domain.KnowledgePage

	page.Page, page.PageSize = q.Page, q.PageSize
	if err := r.pool.QueryRow(ctx, "SELECT count(*) FROM knowledge k WHERE "+whereSQL, args...).
		Scan(&page.Total); err != nil {
		return page, dbError(err)
	}

	offset := (q.Page - 1) * q.PageSize
	args = append(args, q.PageSize, offset)

	rows, err := r.pool.Query(
		ctx,
		`SELECT `+knowledgeColumns+` FROM knowledge k LEFT JOIN knowledge_folders f ON f.id=k.folder_id WHERE `+whereSQL+` ORDER BY k.updated_at DESC,k.id DESC LIMIT $`+fmt.Sprint(
			arg,
		)+` OFFSET $`+fmt.Sprint(
			arg+1,
		),
		args...)
	if err != nil {
		return page, dbError(err)
	}
	defer rows.Close()

	page.Items = []domain.Knowledge{}

	for rows.Next() {
		k, err := scanKnowledge(rows)
		if err != nil {
			return page, err
		}

		page.Items = append(page.Items, k)
	}

	return page, dbError(rows.Err())
}

func (r *KnowledgeRepository) SearchTextBackfill(
	ctx context.Context,
	owner string,
) ([]domain.Knowledge, error) {
	rows, err := r.pool.Query(
		ctx,
		`SELECT id::text,title,tags,source_key FROM knowledge WHERE owner_id=$1 AND search_text_ready=false ORDER BY id LIMIT 100`,
		owner,
	)
	if err != nil {
		return nil, dbError(err)
	}
	defer rows.Close()

	items := []domain.Knowledge{}

	for rows.Next() {
		var item domain.Knowledge
		if err := rows.Scan(&item.ID, &item.Title, &item.Tags, &item.SourceKey); err != nil {
			return nil, dbError(err)
		}

		items = append(items, item)
	}

	return items, dbError(rows.Err())
}

func (r *KnowledgeRepository) SetSearchText(
	ctx context.Context,
	owner, id, searchText string,
) error {
	_, err := r.pool.Exec(
		ctx,
		`UPDATE knowledge SET search_text=$3,search_text_ready=true WHERE id=$1 AND owner_id=$2 AND search_text_ready=false`,
		id,
		owner,
		searchText,
	)

	return dbError(err)
}

// ponytail: evaluate only query lexemes; add a search index only if latency is measured.
func fullTextSearchCondition(arg int, value string) (string, []any) {
	queryArg := "$" + fmt.Sprint(arg)

	return `EXISTS (
		WITH
		search_query AS (
			SELECT regexp_replace(
				websearch_to_tsquery('simple', ` + queryArg + `)::text,
				'<->|<[0-9]+>',
				'&',
				'g'
			)::tsquery AS query
		),
		query_lexemes AS (
			SELECT DISTINCT word.lexeme
			FROM search_query
			CROSS JOIN LATERAL ts_debug('simple', search_query.query::text) AS parsed
			CROSS JOIN LATERAL unnest(parsed.lexemes) AS word(lexeme)
			WHERE word.lexeme IS NOT NULL
		),
		document_lexemes AS (
			SELECT strip(array_to_tsvector(
				COALESCE(array_agg(DISTINCT word.lexeme), ARRAY[]::text[])
			)) AS vector
			FROM ts_debug('simple', k.search_text) AS parsed
			CROSS JOIN LATERAL unnest(parsed.lexemes) AS word(lexeme)
			WHERE word.lexeme IN (SELECT lexeme FROM query_lexemes)
		)
		SELECT 1
		FROM search_query
		CROSS JOIN document_lexemes
		WHERE document_lexemes.vector @@ search_query.query
	)`, []any{value}
}

func scanDraft(row pgx.Row) (domain.Draft, error) {
	var (
		d                                    domain.Draft
		folderID, folderName, folderParentID *string
		folderVersion                        *int64
	)

	err := row.Scan(
		&d.DraftID,
		&d.OwnerID,
		&d.Title,
		&d.Format,
		&d.Tags,
		&d.LearningStatus,
		&folderID,
		&folderName,
		&folderParentID,
		&folderVersion,
		&d.Version,
		&d.SourceKey,
		&d.CommittedID,
		&d.CommitHash,
	)
	if folderID != nil && folderName != nil && folderVersion != nil {
		d.Folder = &domain.Folder{ID: *folderID, Name: *folderName, ParentID: folderParentID, Version: *folderVersion}
	}

	return d, dbError(err)
}

const draftColumns = `d.id::text,d.owner_id,d.title,d.format,d.tags,d.learning_status,f.id::text,f.name,f.parent_id::text,f.version,d.version,d.source_key,COALESCE(d.committed_id::text,''),COALESCE(d.commit_hash,'')`

func (r *KnowledgeRepository) Draft(ctx context.Context, owner, id string) (domain.Draft, error) {
	return scanDraft(
		r.pool.QueryRow(
			ctx,
			`SELECT `+draftColumns+` FROM knowledge_drafts d LEFT JOIN knowledge_folders f ON f.id=d.folder_id WHERE d.id=$1 AND d.owner_id=$2`,
			id,
			owner,
		),
	)
}

func (r *KnowledgeRepository) CreateDraft(ctx context.Context, d domain.Draft) error {
	var folderID any
	if d.Folder != nil {
		folderID = d.Folder.ID
	}

	_, err := r.pool.Exec(
		ctx,
		`INSERT INTO knowledge_drafts(id,owner_id,title,format,tags,learning_status,folder_id,version,source_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		d.DraftID,
		d.OwnerID,
		d.Title,
		d.Format,
		d.Tags,
		d.LearningStatus,
		folderID,
		d.Version,
		d.SourceKey,
	)

	return dbError(err)
}

func (r *KnowledgeRepository) StageObjects(ctx context.Context, keys []string, write func() error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return dbError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, key := range keys {
		if _, err := tx.Exec(ctx, `INSERT INTO knowledge_objects(object_key) VALUES ($1)`, key); err != nil {
			return dbError(err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return dbError(err)
	}
	// 保留行を先に保存し、Put中もロックを保持して回収とアップロードの競合を防ぐ。
	tx, err = r.pool.Begin(ctx)
	if err != nil {
		return dbError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockObjects(ctx, tx, keys); err != nil {
		return err
	}

	if err := write(); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `UPDATE knowledge_objects SET state='ready' WHERE object_key=ANY($1)`, keys); err != nil {
		return dbError(err)
	}

	return dbError(tx.Commit(ctx))
}

func lockObjects(ctx context.Context, tx pgx.Tx, keys []string) error {
	sort.Strings(keys)

	for _, key := range keys {
		var state string
		if err := tx.QueryRow(ctx, `SELECT state FROM knowledge_objects WHERE object_key=$1 FOR UPDATE`, key).
			Scan(&state); err != nil {
			return dbError(err)
		}
	}

	return nil
}

func activate(ctx context.Context, tx pgx.Tx, k domain.Knowledge) error {
	keys := []string{k.SourceKey}
	if k.HTMLKey != "" {
		keys = append(keys, k.HTMLKey)
	}

	if err := lockObjects(ctx, tx, keys); err != nil {
		return err
	}

	tag, err := tx.Exec(
		ctx,
		`UPDATE knowledge_objects SET state='active' WHERE object_key=ANY($1) AND state='ready'`,
		keys,
	)
	if err != nil {
		return dbError(err)
	}

	if tag.RowsAffected() != int64(len(keys)) {
		return domain.ErrConflict
	}

	return nil
}

func audit(ctx context.Context, tx pgx.Tx, k domain.Knowledge, action string) error {
	_, err := tx.Exec(
		ctx,
		`INSERT INTO knowledge_audit(knowledge_id,owner_id,action,version) VALUES($1,$2,$3,$4)`,
		k.ID,
		k.OwnerID,
		action,
		k.Version,
	)

	return dbError(err)
}

func (r *KnowledgeRepository) Save(ctx context.Context, k domain.Knowledge, version int64) (domain.Knowledge, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return k, dbError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := scanKnowledge(
		tx.QueryRow(
			ctx,
			`SELECT `+knowledgeColumns+` FROM knowledge k LEFT JOIN knowledge_folders f ON f.id=k.folder_id WHERE k.id=$1 AND k.owner_id=$2 FOR UPDATE OF k`,
			k.ID,
			k.OwnerID,
		),
	)
	if err != nil {
		return k, err
	}

	if current.Version != version || version >= domain.MaxVersion {
		return k, domain.ErrConflict
	}

	if err := activate(ctx, tx, k); err != nil {
		return k, err
	}

	err = tx.QueryRow(ctx, `UPDATE knowledge SET title=$2,tags=$3,learning_status=$4,source_key=$5,html_key=NULLIF($6,''),html_sanitized=$7,search_text=$8,search_text_ready=true,version=version+1,updated_at=clock_timestamp() WHERE id=$1 RETURNING version,updated_at`, k.ID, k.Title, k.Tags, k.LearningStatus, k.SourceKey, k.HTMLKey, k.HTMLSanitized, k.SearchText).
		Scan(&k.Version, &k.UpdatedAt)
	if err != nil {
		return k, dbError(err)
	}

	k.Visibility = current.Visibility

	k.PublicID = current.PublicID
	if err := audit(ctx, tx, k, "save"); err != nil {
		return k, err
	}

	return k, dbError(tx.Commit(ctx))
}

func (r *KnowledgeRepository) Commit(
	ctx context.Context,
	d domain.Draft,
	k domain.Knowledge,
	hash string,
) (string, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", false, dbError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := scanDraft(
		tx.QueryRow(
			ctx,
			`SELECT `+draftColumns+` FROM knowledge_drafts d LEFT JOIN knowledge_folders f ON f.id=d.folder_id WHERE d.id=$1 AND d.owner_id=$2 FOR UPDATE OF d`,
			d.DraftID,
			d.OwnerID,
		),
	)
	if err != nil {
		return "", false, err
	}

	if current.Version != d.Version {
		return "", false, domain.ErrConflict
	}

	if current.CommittedID != "" {
		if current.CommitHash == hash {
			return current.CommittedID, false, nil
		}

		return "", false, domain.ErrConflict
	}

	if err := activate(ctx, tx, k); err != nil {
		return "", false, err
	}

	var folderID any
	if current.Folder != nil {
		folderID = current.Folder.ID
	}

	_, err = tx.Exec(
		ctx,
		`INSERT INTO knowledge(id,owner_id,title,format,tags,learning_status,folder_id,source_key,html_key,html_sanitized,search_text,search_text_ready) VALUES($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''),$10,$11,true)`,
		k.ID,
		k.OwnerID,
		k.Title,
		k.Format,
		k.Tags,
		k.LearningStatus,
		folderID,
		k.SourceKey,
		k.HTMLKey,
		k.HTMLSanitized,
		k.SearchText,
	)
	if err != nil {
		return "", false, dbError(err)
	}

	_, err = tx.Exec(
		ctx,
		`UPDATE knowledge_drafts SET committed_id=$2,commit_hash=$3 WHERE id=$1`,
		d.DraftID,
		k.ID,
		hash,
	)
	if err != nil {
		return "", false, dbError(err)
	}

	k.Version = 1
	if err := audit(ctx, tx, k, "commit"); err != nil {
		return "", false, err
	}

	return k.ID, true, dbError(tx.Commit(ctx))
}

func (r *KnowledgeRepository) Visibility(
	ctx context.Context,
	owner, id string,
	version int64,
	visibility, publicID string,
) (domain.Knowledge, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Knowledge{}, dbError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	k, err := scanKnowledge(
		tx.QueryRow(
			ctx,
			`SELECT `+knowledgeColumns+` FROM knowledge k LEFT JOIN knowledge_folders f ON f.id=k.folder_id WHERE k.id=$1 AND k.owner_id=$2 FOR UPDATE OF k`,
			id,
			owner,
		),
	)
	if err != nil {
		return k, err
	}

	if k.Version != version {
		return k, domain.ErrConflict
	}

	if k.Visibility == visibility {
		return k, nil
	}

	if version >= domain.MaxVersion {
		return k, domain.ErrConflict
	}

	if k.PublicID == nil && visibility == "unlisted" {
		k.PublicID = &publicID
	}

	k.Visibility = visibility

	err = tx.QueryRow(ctx, `UPDATE knowledge SET visibility=$2,public_id=$3,version=version+1,updated_at=clock_timestamp() WHERE id=$1 RETURNING version,updated_at`, id, visibility, k.PublicID).
		Scan(&k.Version, &k.UpdatedAt)
	if err != nil {
		return k, dbError(err)
	}

	if err := audit(ctx, tx, k, "visibility"); err != nil {
		return k, err
	}

	return k, dbError(tx.Commit(ctx))
}

func (r *KnowledgeRepository) Grant(ctx context.Context, g domain.PreviewGrant) error {
	_, err := r.pool.Exec(
		ctx,
		`INSERT INTO knowledge_preview_grants(hash,owner_id,session_id,knowledge_id,version) VALUES($1,$2,$3,$4,$5)`,
		g.Hash,
		g.OwnerID,
		g.SessionID,
		g.KnowledgeID,
		g.Version,
	)

	return dbError(err)
}

func (r *KnowledgeRepository) Preview(ctx context.Context, hash string) (domain.PreviewGrant, error) {
	var g domain.PreviewGrant

	err := r.pool.QueryRow(ctx, `SELECT hash,owner_id,session_id,knowledge_id::text,version FROM knowledge_preview_grants WHERE hash=$1`, hash).
		Scan(&g.Hash, &g.OwnerID, &g.SessionID, &g.KnowledgeID, &g.Version)

	return g, dbError(err)
}

func (r *KnowledgeRepository) Public(ctx context.Context, id string, version int64) (domain.Knowledge, error) {
	return scanKnowledge(
		r.pool.QueryRow(
			ctx,
			`SELECT `+knowledgeColumns+` FROM knowledge k LEFT JOIN knowledge_folders f ON f.id=k.folder_id WHERE k.public_id=$1 AND k.visibility='unlisted' AND k.version=$2`,
			id,
			version,
		),
	)
}

func (r *KnowledgeRepository) Folders(ctx context.Context, owner string) ([]domain.Folder, error) {
	rows, err := r.pool.Query(
		ctx,
		`SELECT id::text,name,parent_id::text,version FROM knowledge_folders WHERE owner_id=$1 ORDER BY parent_id NULLS FIRST,name,id`,
		owner,
	)
	if err != nil {
		return nil, dbError(err)
	}
	defer rows.Close()

	folders := []domain.Folder{}

	for rows.Next() {
		var folder domain.Folder
		if err := rows.Scan(&folder.ID, &folder.Name, &folder.ParentID, &folder.Version); err != nil {
			return nil, dbError(err)
		}

		folders = append(folders, folder)
	}

	return folders, dbError(rows.Err())
}

func (r *KnowledgeRepository) CreateFolder(
	ctx context.Context,
	owner, name string,
	parentID *string,
) (domain.Folder, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Folder{}, dbError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockOwner(ctx, tx, owner); err != nil {
		return domain.Folder{}, err
	}

	var duplicate bool
	if err := tx.QueryRow(
		ctx,
		`SELECT EXISTS(SELECT 1 FROM knowledge_folders WHERE owner_id=$1 AND name=$2 AND parent_id IS NOT DISTINCT FROM $3)`,
		owner,
		name,
		parentID,
	).Scan(&duplicate); err != nil {
		return domain.Folder{}, dbError(err)
	}

	if duplicate {
		return domain.Folder{}, domain.ErrConflict
	}

	id := newUUID()

	_, err = tx.Exec(
		ctx,
		`INSERT INTO knowledge_folders(id,owner_id,name,parent_id) VALUES($1,$2,$3,$4)`,
		id,
		owner,
		name,
		parentID,
	)
	if err != nil {
		return domain.Folder{}, dbError(err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Folder{}, dbError(err)
	}

	return domain.Folder{ID: id, Name: name, ParentID: parentID, Version: 1}, nil
}

func lockOwner(ctx context.Context, tx pgx.Tx, owner string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, owner)
	return dbError(err)
}

func (r *KnowledgeRepository) UpdateFolder(
	ctx context.Context,
	owner, id, name string,
	parentID *string,
	parentSpecified bool,
	version int64,
) (domain.Folder, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Folder{}, dbError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockOwner(ctx, tx, owner); err != nil {
		return domain.Folder{}, err
	}

	var current domain.Folder
	if err := tx.QueryRow(ctx, `SELECT id::text,name,parent_id::text,version FROM knowledge_folders WHERE id=$1 AND owner_id=$2 FOR UPDATE`, id, owner).
		Scan(&current.ID, &current.Name, &current.ParentID, &current.Version); err != nil {
		return domain.Folder{}, dbError(err)
	}

	if current.Version != version || version >= domain.MaxVersion {
		return domain.Folder{}, domain.ErrConflict
	}

	newParent := current.ParentID
	if parentSpecified {
		newParent = parentID
	}

	if newParent != nil {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM knowledge_folders WHERE id=$1 AND owner_id=$2)`, *newParent, owner).
			Scan(&exists); err != nil {
			return domain.Folder{}, dbError(err)
		}

		if !exists {
			return domain.Folder{}, domain.ErrNotFound
		}

		var cycle bool
		if err := tx.QueryRow(ctx, `WITH RECURSIVE descendants(id) AS (SELECT id FROM knowledge_folders WHERE id=$1 AND owner_id=$2 UNION SELECT f.id FROM knowledge_folders f JOIN descendants d ON f.parent_id=d.id WHERE f.owner_id=$2) SELECT EXISTS(SELECT 1 FROM descendants WHERE id=$3)`, id, owner, *newParent).
			Scan(&cycle); err != nil {
			return domain.Folder{}, dbError(err)
		}

		if cycle {
			return domain.Folder{}, &domain.ValidationError{
				Detail: domain.FieldError{Field: "parentId", Reason: "cycle"},
			}
		}
	}

	var duplicate bool
	if err := tx.QueryRow(
		ctx,
		`SELECT EXISTS(SELECT 1 FROM knowledge_folders WHERE owner_id=$1 AND name=$2 AND parent_id IS NOT DISTINCT FROM $3 AND id<>$4)`,
		owner,
		name,
		newParent,
		id,
	).Scan(&duplicate); err != nil {
		return domain.Folder{}, dbError(err)
	}

	if duplicate {
		return domain.Folder{}, domain.ErrConflict
	}

	if _, err := tx.Exec(
		ctx,
		`UPDATE knowledge_folders SET name=$3,parent_id=$4,version=version+1 WHERE id=$1 AND owner_id=$2`,
		id,
		owner,
		name,
		newParent,
	); err != nil {
		return domain.Folder{}, dbError(err)
	}

	current.Name, current.ParentID, current.Version = name, newParent, version+1

	if err := tx.Commit(ctx); err != nil {
		return domain.Folder{}, dbError(err)
	}

	return current, nil
}

func (r *KnowledgeRepository) DeleteFolder(ctx context.Context, owner, id string, version int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return dbError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockOwner(ctx, tx, owner); err != nil {
		return err
	}

	var currentVersion int64
	if err := tx.QueryRow(ctx, `SELECT version FROM knowledge_folders WHERE id=$1 AND owner_id=$2 FOR UPDATE`, id, owner).
		Scan(&currentVersion); err != nil {
		return dbError(err)
	}

	if currentVersion != version {
		return domain.ErrConflict
	}

	var nonEmpty bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM knowledge k WHERE k.owner_id=$1 AND k.folder_id=$2) OR EXISTS(SELECT 1 FROM knowledge_drafts d WHERE d.owner_id=$1 AND d.folder_id=$2) OR EXISTS(SELECT 1 FROM knowledge_folders f WHERE f.owner_id=$1 AND f.parent_id=$2)`, owner, id).
		Scan(&nonEmpty); err != nil {
		return dbError(err)
	}

	if nonEmpty {
		return domain.ErrConflict
	}

	if _, err := tx.Exec(ctx, `DELETE FROM knowledge_folders WHERE id=$1 AND owner_id=$2`, id, owner); err != nil {
		return dbError(err)
	}

	return dbError(tx.Commit(ctx))
}

func (r *KnowledgeRepository) MoveKnowledge(
	ctx context.Context,
	owner, id string,
	folderID *string,
	version int64,
) (domain.Knowledge, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Knowledge{}, dbError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockOwner(ctx, tx, owner); err != nil {
		return domain.Knowledge{}, err
	}

	current, err := scanKnowledge(
		tx.QueryRow(
			ctx,
			`SELECT `+knowledgeColumns+` FROM knowledge k LEFT JOIN knowledge_folders f ON f.id=k.folder_id WHERE k.id=$1 AND k.owner_id=$2 FOR UPDATE OF k`,
			id,
			owner,
		),
	)
	if err != nil {
		return domain.Knowledge{}, err
	}

	if current.Version != version || version >= domain.MaxVersion {
		return domain.Knowledge{}, domain.ErrConflict
	}

	if folderID != nil {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM knowledge_folders WHERE id=$1 AND owner_id=$2)`, *folderID, owner).
			Scan(&exists); err != nil {
			return domain.Knowledge{}, dbError(err)
		}

		if !exists {
			return domain.Knowledge{}, domain.ErrNotFound
		}
	}

	if _, err := tx.Exec(
		ctx,
		`UPDATE knowledge SET folder_id=$3,version=version+1,updated_at=clock_timestamp() WHERE id=$1 AND owner_id=$2`,
		id,
		owner,
		folderID,
	); err != nil {
		return domain.Knowledge{}, dbError(err)
	}

	updated, err := scanKnowledge(
		tx.QueryRow(
			ctx,
			`SELECT `+knowledgeColumns+` FROM knowledge k LEFT JOIN knowledge_folders f ON f.id=k.folder_id WHERE k.id=$1 AND k.owner_id=$2`,
			id,
			owner,
		),
	)
	if err != nil {
		return domain.Knowledge{}, err
	}

	if err := audit(ctx, tx, updated, "move"); err != nil {
		return domain.Knowledge{}, err
	}

	return updated, dbError(tx.Commit(ctx))
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// Collect は基準時刻より古い未参照オブジェクトだけを削除する。S3の削除処理は呼び出し元が渡す。
// 行ロックでアップロード・保存確定中の対象を除外する。削除に失敗した場合は再試行できる。
func (r *KnowledgeRepository) Collect(
	ctx context.Context,
	before time.Time,
	remove func(context.Context, string) error,
) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, dbError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(
		ctx,
		`SELECT object_key FROM knowledge_objects o WHERE created_at<$1 AND NOT EXISTS(SELECT 1 FROM knowledge k WHERE k.source_key=o.object_key OR k.html_key=o.object_key) AND NOT EXISTS(SELECT 1 FROM knowledge_drafts d WHERE d.source_key=o.object_key) ORDER BY object_key FOR UPDATE SKIP LOCKED LIMIT 100`,
		before,
	)
	if err != nil {
		return 0, dbError(err)
	}

	var keys []string

	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return 0, dbError(err)
		}

		keys = append(keys, key)
	}

	rows.Close()

	if err := rows.Err(); err != nil {
		return 0, dbError(err)
	}

	removed := 0

	for _, key := range keys {
		var referenced bool

		err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM knowledge WHERE source_key=$1 OR html_key=$1) OR EXISTS(SELECT 1 FROM knowledge_drafts WHERE source_key=$1)`, key).
			Scan(&referenced)
		if err != nil {
			return 0, dbError(err)
		}

		if referenced {
			continue
		}

		if err := remove(ctx, key); err != nil {
			return 0, err
		}

		if _, err := tx.Exec(ctx, `DELETE FROM knowledge_objects WHERE object_key=$1`, key); err != nil {
			return 0, dbError(err)
		}

		removed++
	}

	return removed, dbError(tx.Commit(ctx))
}
