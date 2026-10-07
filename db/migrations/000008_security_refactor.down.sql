ALTER TABLE "user" RENAME COLUMN authentication_snapshot_version TO credential_version;

ALTER TABLE "user" RENAME CONSTRAINT user_authentication_snapshot_version_not_nil
    TO user_credential_version_not_nil;

ALTER TABLE "user" RENAME CONSTRAINT user_authentication_snapshot_version_not_null
    TO user_credential_version_not_null;

ALTER TRIGGER rotate_user_authentication_snapshot_version ON "user"
    RENAME TO rotate_user_credential_version;

ALTER FUNCTION rotate_user_authentication_snapshot_version()
    RENAME TO rotate_user_credential_version;

CREATE OR REPLACE FUNCTION rotate_user_credential_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.password_hash IS DISTINCT FROM OLD.password_hash
       OR NEW.email IS DISTINCT FROM OLD.email THEN
        NEW.credential_version := uuidv7();
    END IF;
    RETURN NEW;
END;
$$;
