CREATE TABLE document_presentation (
 document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
 namespace TEXT NOT NULL,
 object_id TEXT NOT NULL,
 version INTEGER NOT NULL CHECK(version>0),
 payload TEXT NOT NULL,
 PRIMARY KEY(document_id,namespace,object_id)
);
