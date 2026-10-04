package database

// Equivalent to the PostgreSQL role functions in migration 0006. Keeping the
// lookup within the SQL statement preserves its transaction snapshot and avoids
// re-entering the pool from a SQLite user-defined function.
const sqliteFolderRole = `(WITH RECURSIVE acl_folder_input AS (
 SELECT %s AS target, %s AS principal
), acl_ancestors AS (
 SELECT f.id,f.parent_id,f.owner_user_id FROM folders f,acl_folder_input i WHERE f.id=i.target
 UNION ALL
 SELECT f.id,f.parent_id,f.owner_user_id FROM folders f JOIN acl_ancestors a ON f.id=a.parent_id
), acl_folder_candidates AS (
 SELECT CASE WHEN a.owner_user_id=i.principal THEN 30 ELSE 0 END AS level FROM acl_ancestors a,acl_folder_input i
 UNION ALL
 SELECT role_level(g.role) FROM resource_grants g JOIN acl_ancestors a ON g.resource_type='FOLDER' AND g.resource_id=a.id
 CROSS JOIN acl_folder_input i
 WHERE g.user_id=i.principal OR EXISTS(SELECT 1 FROM team_members tm WHERE tm.team_id=g.team_id AND tm.user_id=i.principal)
) SELECT COALESCE(max(level),0) FROM acl_folder_candidates)`

const sqliteDocumentRole = `(WITH acl_document_input AS (
 SELECT %s AS target, %s AS principal
), acl_document_data AS (
 SELECT d.owner_user_id,d.folder_id FROM documents d,acl_document_input i WHERE d.id=i.target
), acl_document_candidates AS (
 SELECT CASE WHEN d.owner_user_id=i.principal THEN 30 ELSE 0 END AS level FROM acl_document_data d,acl_document_input i
 UNION ALL
 SELECT effective_folder_role(d.folder_id,i.principal) FROM acl_document_data d,acl_document_input i WHERE d.folder_id IS NOT NULL
 UNION ALL
 SELECT role_level(g.role) FROM resource_grants g,acl_document_input i
 WHERE g.resource_type='DOCUMENT' AND g.resource_id=i.target
 AND (g.user_id=i.principal OR EXISTS(SELECT 1 FROM team_members tm WHERE tm.team_id=g.team_id AND tm.user_id=i.principal))
) SELECT COALESCE(max(level),0) FROM acl_document_candidates)`
