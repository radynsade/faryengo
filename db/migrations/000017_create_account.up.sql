CREATE TABLE "account" (
    id uuid PRIMARY KEY,
    workspace_id uuid NOT NULL,
    user_id uuid,
    name varchar(100) NOT NULL,
    CONSTRAINT account_id_not_nil CHECK (id <> '00000000-0000-0000-0000-000000000000'::uuid),
    CONSTRAINT account_user_id_not_nil CHECK (user_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    CONSTRAINT account_name_not_blank CHECK (name ~ '[^[:space:]]'),
    CONSTRAINT account_workspace_id_fkey FOREIGN KEY (workspace_id)
        REFERENCES "workspace" (id) ON DELETE CASCADE,
    -- An account outlives its user and is left without one.
    CONSTRAINT account_user_id_fkey FOREIGN KEY (user_id)
        REFERENCES "user" (id) ON DELETE SET NULL
);

-- These indexes serve listing a workspace's or a user's accounts and the
-- referential actions when a workspace or a user is deleted.
CREATE INDEX account_workspace_id_idx ON "account" (workspace_id);
CREATE INDEX account_user_id_idx ON "account" (user_id);
