CREATE TABLE occccad.document_presentation (
 document_id UUID NOT NULL REFERENCES occccad.documents(id) ON DELETE CASCADE,
 namespace TEXT NOT NULL,
 object_id TEXT NOT NULL,
 version BIGINT NOT NULL CHECK(version>0),
 payload JSONB NOT NULL,
 PRIMARY KEY(document_id,namespace,object_id)
);
