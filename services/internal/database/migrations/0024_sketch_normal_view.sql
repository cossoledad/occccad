-- View-only sketch navigation, available in the same catalog for API and mock clients.
INSERT INTO occccad.ui_toolbar_items(toolbar_id,command_id,name,help_text,icon_key,group_key,sort_order,repeatable)
VALUES ('sketch-lifecycle','sketch.normal','正对草图平面','恢复活动草图平面的正视方向，保留当前缩放和关注区域。','top-view','primary',20,false)
ON CONFLICT (toolbar_id,command_id) DO NOTHING;
