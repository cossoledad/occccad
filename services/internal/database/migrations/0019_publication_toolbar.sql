-- P9: expose the stable Publication contract manager in Part Design.
INSERT INTO occccad.ui_toolbar_items(
    toolbar_id,command_id,name,help_text,icon_key,sort_order,repeatable
) VALUES (
    'part-design','part.publications','发布',
    '创建和管理可跨文档引用的稳定几何与参数契约。','parameters',27,false
)
ON CONFLICT (toolbar_id,command_id) DO UPDATE SET
    name=EXCLUDED.name,
    help_text=EXCLUDED.help_text,
    icon_key=EXCLUDED.icon_key,
    sort_order=EXCLUDED.sort_order,
    repeatable=EXCLUDED.repeatable;
