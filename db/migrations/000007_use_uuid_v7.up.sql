ALTER TABLE "user" ALTER COLUMN credential_version SET DEFAULT uuidv7();

CREATE OR REPLACE FUNCTION rotate_user_credential_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.password_hash IS DISTINCT FROM OLD.password_hash
       OR NEW.email IS DISTINCT FROM OLD.email THEN
        NEW.credential_version := uuidv7();
    END IF;
    RETURN NEW;
END;
$$;
