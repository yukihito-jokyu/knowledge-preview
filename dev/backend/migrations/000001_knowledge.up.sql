-- 認証情報とセッションは#19で管理する。owner_idは認証済みの識別子として扱い、内部構造を解釈しない。
CREATE TABLE knowledge_folders (
 id uuid PRIMARY KEY, owner_id text NOT NULL, name text NOT NULL,
 UNIQUE (id, owner_id)
);
CREATE TABLE knowledge_objects (
 object_key text PRIMARY KEY,
 state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'ready', 'active')),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE knowledge (
 id uuid PRIMARY KEY,
 owner_id text NOT NULL,
 title text NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
 format text NOT NULL CHECK (format IN ('markdown', 'html')),
 tags text[] NOT NULL DEFAULT '{}',
 learning_status text NOT NULL DEFAULT 'unlearned' CHECK (learning_status IN ('unlearned', 'learning', 'learned')),
 folder_id uuid,
 version bigint NOT NULL DEFAULT 1 CHECK (version BETWEEN 1 AND 9007199254740991),
 updated_at timestamptz NOT NULL DEFAULT now(),
 visibility text NOT NULL DEFAULT 'private' CHECK (visibility IN ('private', 'unlisted')),
 public_id text UNIQUE,
 source_key text NOT NULL REFERENCES knowledge_objects(object_key),
 html_key text REFERENCES knowledge_objects(object_key),
 html_sanitized boolean NOT NULL DEFAULT false,
 FOREIGN KEY (folder_id, owner_id) REFERENCES knowledge_folders(id, owner_id),
 CHECK (visibility = 'private' OR public_id IS NOT NULL),
 CHECK ((format = 'html') = (html_key IS NOT NULL))
);
CREATE INDEX knowledge_recent ON knowledge (owner_id, updated_at DESC, id ASC);
-- expires_atやTTLは設けず、未確定のdraftデータと正式保存後のID対応を永続保持する。
CREATE TABLE knowledge_drafts (
 id uuid PRIMARY KEY,
 owner_id text NOT NULL,
 title text NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
 format text NOT NULL CHECK (format IN ('markdown', 'html')),
 tags text[] NOT NULL DEFAULT '{}',
 learning_status text NOT NULL DEFAULT 'unlearned' CHECK (learning_status IN ('unlearned', 'learning', 'learned')),
 folder_id uuid,
 version bigint NOT NULL DEFAULT 1 CHECK (version = 1),
 source_key text NOT NULL REFERENCES knowledge_objects(object_key),
 committed_id uuid UNIQUE REFERENCES knowledge(id),
 commit_hash text,
 FOREIGN KEY (folder_id, owner_id) REFERENCES knowledge_folders(id, owner_id),
 CHECK ((committed_id IS NULL) = (commit_hash IS NULL))
);
CREATE TABLE knowledge_preview_grants (
 hash text PRIMARY KEY,
 owner_id text NOT NULL,
 session_id text NOT NULL,
 knowledge_id uuid NOT NULL REFERENCES knowledge(id) ON DELETE CASCADE,
 version bigint NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE knowledge_audit (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 knowledge_id uuid NOT NULL,
 owner_id text NOT NULL,
 action text NOT NULL,
 version bigint NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
