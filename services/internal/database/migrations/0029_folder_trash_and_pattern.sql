-- Folder trash preserves the tree; document revisions and independent trash state stay untouched.
ALTER TABLE occccad.folders
    ADD COLUMN IF NOT EXISTS deleted_at timestamptz,
    ADD COLUMN IF NOT EXISTS trashed_by_folder_id uuid;
CREATE INDEX IF NOT EXISTS folders_trash_roots_idx ON occccad.folders(deleted_at DESC)
    WHERE deleted_at IS NOT NULL AND trashed_by_folder_id = id;
DROP INDEX IF EXISTS occccad.folders_parent_name_idx;
CREATE UNIQUE INDEX folders_parent_name_idx ON occccad.folders(parent_id, lower(name)) NULLS NOT DISTINCT
    WHERE deleted_at IS NULL;

INSERT INTO occccad.ui_toolbar_items(toolbar_id,command_id,name,help_text,icon_key,group_key,sort_order,repeatable)
VALUES ('product-structure','product.pattern','多实例化','沿坐标轴重复所选组件并实时预览。','pattern','primary',20,false)
ON CONFLICT (toolbar_id,command_id) DO NOTHING;
