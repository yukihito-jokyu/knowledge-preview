DROP INDEX knowledge_tags;
DROP INDEX knowledge_search_text;
DROP INDEX knowledge_list_order;
ALTER TABLE knowledge DROP COLUMN search_text;
ALTER TABLE knowledge DROP COLUMN search_text_ready;
DROP INDEX knowledge_folders_owner_parent_name;
ALTER TABLE knowledge_folders
    DROP CONSTRAINT knowledge_folders_not_self_parent,
    DROP CONSTRAINT knowledge_folders_parent_owner_fk,
    DROP COLUMN version,
    DROP COLUMN parent_id;
