DROP TRIGGER rotate_user_credential_version ON "user";
DROP FUNCTION rotate_user_credential_version();
ALTER TABLE "user" DROP COLUMN credential_version;
DROP INDEX user_email_unique_idx;
