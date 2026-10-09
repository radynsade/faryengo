ALTER TABLE "user" RENAME COLUMN credential_version TO authentication_snapshot_version;

ALTER TABLE "user" RENAME CONSTRAINT user_credential_version_not_nil
    TO user_authentication_snapshot_version_not_nil;

ALTER TABLE "user" RENAME CONSTRAINT user_credential_version_not_null
    TO user_authentication_snapshot_version_not_null;

ALTER TRIGGER rotate_user_credential_version ON "user"
    RENAME TO rotate_user_authentication_snapshot_version;

ALTER FUNCTION rotate_user_credential_version()
    RENAME TO rotate_user_authentication_snapshot_version;

CREATE OR REPLACE FUNCTION rotate_user_authentication_snapshot_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.password_hash IS DISTINCT FROM OLD.password_hash
       OR NEW.email IS DISTINCT FROM OLD.email THEN
        NEW.authentication_snapshot_version := uuidv7();
    END IF;
    RETURN NEW;
END;
$$;
