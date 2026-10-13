-- Every member so far could do everything, so existing members become owners.
-- The default only fills those rows; a new membership always names its role.
ALTER TABLE "workspace_membership"
    ADD COLUMN role varchar(16) NOT NULL DEFAULT 'owner',
    ADD CONSTRAINT workspace_membership_role_valid CHECK (role IN ('viewer', 'editor', 'owner'));

ALTER TABLE "workspace_membership" ALTER COLUMN role DROP DEFAULT;
