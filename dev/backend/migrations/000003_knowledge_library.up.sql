ALTER TABLE knowledge_folders
    ADD COLUMN parent_id uuid,
    ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK (version BETWEEN 1 AND 9007199254740991);

ALTER TABLE knowledge_folders
    ADD CONSTRAINT knowledge_folders_parent_owner_fk
        FOREIGN KEY (parent_id, owner_id) REFERENCES knowledge_folders(id, owner_id),
    ADD CONSTRAINT knowledge_folders_not_self_parent CHECK (parent_id IS NULL OR parent_id <> id);

-- Existing installs may contain same-owner/root duplicates from 000001. Keep them
-- intact; the application enforces uniqueness under an owner transaction lock.
CREATE INDEX knowledge_folders_owner_parent_name
    ON knowledge_folders (owner_id, COALESCE(parent_id, '00000000-0000-0000-0000-000000000000'::uuid), name);

ALTER TABLE knowledge
    ADD COLUMN search_text text NOT NULL DEFAULT '';
ALTER TABLE knowledge
    ADD COLUMN search_text_ready boolean NOT NULL DEFAULT false;

CREATE INDEX knowledge_list_order ON knowledge (owner_id, updated_at DESC, id DESC);
CREATE INDEX knowledge_search_text ON knowledge USING gin (to_tsvector('simple', left(search_text, 100000)));
CREATE INDEX knowledge_tags ON knowledge USING gin (tags);
