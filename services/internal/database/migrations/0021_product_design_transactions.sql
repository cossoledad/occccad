CREATE TABLE occccad.product_design_transactions (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    root_product_document_id uuid NOT NULL REFERENCES occccad.documents(id) ON DELETE CASCADE,
    actor_id uuid NOT NULL REFERENCES occccad.users(id) ON DELETE RESTRICT,
    request_id text NOT NULL,
    request_digest text NOT NULL,
    status text NOT NULL CHECK (status IN ('COMMITTED','REJECTED','CONFLICT','FAILED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    committed_at timestamptz,
    UNIQUE (root_product_document_id,request_id)
);

ALTER TABLE occccad.domain_transactions
    ADD COLUMN product_design_transaction_id uuid
        REFERENCES occccad.product_design_transactions(id) ON DELETE RESTRICT;

CREATE INDEX domain_transactions_product_design_idx
    ON occccad.domain_transactions(product_design_transaction_id)
    WHERE product_design_transaction_id IS NOT NULL;
