ALTER TABLE "user" ALTER COLUMN credential_version SET DEFAULT gen_random_uuid();

CREATE OR REPLACE FUNCTION rotate_user_credential_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.password_hash IS DISTINCT FROM OLD.password_hash
       OR NEW.email IS DISTINCT FROM OLD.email THEN
        NEW.credential_version := gen_random_uuid();
    END IF;
    RETURN NEW;
END;
$$;
