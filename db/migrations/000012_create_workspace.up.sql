CREATE TABLE "workspace" (
    id uuid PRIMARY KEY,
    name varchar(100) NOT NULL,
    CONSTRAINT workspace_id_not_nil CHECK (id <> '00000000-0000-0000-0000-000000000000'::uuid),
    CONSTRAINT workspace_name_not_blank CHECK (name ~ '[^[:space:]]')
);

CREATE TABLE "workspace_membership" (
    workspace_id uuid NOT NULL,
    user_id uuid NOT NULL,
    CONSTRAINT workspace_membership_pkey PRIMARY KEY (workspace_id, user_id),
    CONSTRAINT workspace_membership_workspace_id_fkey FOREIGN KEY (workspace_id)
        REFERENCES "workspace" (id) ON DELETE CASCADE,
    CONSTRAINT workspace_membership_user_id_fkey FOREIGN KEY (user_id)
        REFERENCES "user" (id) ON DELETE CASCADE
);

-- The primary key serves lookups by workspace; this index serves listing a
-- user's workspaces and the cascade when a user is deleted.
CREATE INDEX workspace_membership_user_id_idx ON "workspace_membership" (user_id);
