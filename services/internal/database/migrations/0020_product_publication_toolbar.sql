-- P9E: Product forwards child Publications through the same contract manager.
INSERT INTO occccad.ui_toolbar_items(
    toolbar_id,command_id,name,help_text,icon_key,sort_order,repeatable
) VALUES (
    'assembly-design','product.publications','发布',
    '转发子 Part Publication，形成稳定 Product 装配接口。','parameters',25,false
)
ON CONFLICT (toolbar_id,command_id) DO UPDATE SET
    name=EXCLUDED.name,
    help_text=EXCLUDED.help_text,
    icon_key=EXCLUDED.icon_key,
    sort_order=EXCLUDED.sort_order,
    repeatable=EXCLUDED.repeatable;
