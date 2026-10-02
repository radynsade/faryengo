-- Existing duplicate email addresses must be resolved before applying this migration.
CREATE UNIQUE INDEX user_email_unique_idx ON "user" (lower(email));

ALTER TABLE "user" ADD COLUMN credential_version uuid NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE "user" ADD CONSTRAINT user_credential_version_not_nil
    CHECK (credential_version <> '00000000-0000-0000-0000-000000000000'::uuid);

CREATE FUNCTION rotate_user_credential_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.password_hash IS DISTINCT FROM OLD.password_hash
       OR NEW.email IS DISTINCT FROM OLD.email THEN
        NEW.credential_version := gen_random_uuid();
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER rotate_user_credential_version
BEFORE UPDATE ON "user"
FOR EACH ROW EXECUTE FUNCTION rotate_user_credential_version();
