-- Expose the Part-level parameter manager without requiring a selected feature.
INSERT INTO occccad.ui_toolbar_items(
    toolbar_id,command_id,name,help_text,icon_key,sort_order,repeatable
) VALUES (
    'part-design','part.parameters','参数',
    '集中查看、编辑并复用当前 Part 的参数。','parameters',25,false
)
ON CONFLICT (toolbar_id,command_id) DO UPDATE SET
    name=EXCLUDED.name,
    help_text=EXCLUDED.help_text,
    icon_key=EXCLUDED.icon_key,
    sort_order=EXCLUDED.sort_order,
    repeatable=EXCLUDED.repeatable;
