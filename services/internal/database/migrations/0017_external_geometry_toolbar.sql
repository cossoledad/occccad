-- Make the P8 ExternalGeometry projection command available to databases that
-- already applied the original toolbar catalog migration.
INSERT INTO occccad.ui_toolbar_items(
    toolbar_id,command_id,name,help_text,icon_key,sort_order,repeatable
) VALUES (
    'sketch-geometry','sketch.project','投影',
    '将已有实体的边或顶点关联投影为外部几何。','project',18,true
)
ON CONFLICT (toolbar_id,command_id) DO NOTHING;
