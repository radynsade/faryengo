ALTER TABLE "user"
    ADD COLUMN email_changed_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ADD COLUMN phone_changed_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ADD COLUMN password_changed_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ADD COLUMN updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ADD COLUMN created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP;

CREATE FUNCTION update_user_timestamps() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at := GREATEST(clock_timestamp(), OLD.updated_at + interval '1 microsecond');
    NEW.created_at := OLD.created_at;
    NEW.email_changed_at := OLD.email_changed_at;
    NEW.phone_changed_at := OLD.phone_changed_at;
    NEW.password_changed_at := OLD.password_changed_at;

    IF NEW.email IS DISTINCT FROM OLD.email THEN
        NEW.email_changed_at := NEW.updated_at;
    END IF;

    IF NEW.phone IS DISTINCT FROM OLD.phone THEN
        NEW.phone_changed_at := NEW.updated_at;
    END IF;

    IF NEW.password_hash IS DISTINCT FROM OLD.password_hash THEN
        NEW.password_changed_at := NEW.updated_at;
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER update_user_timestamps BEFORE UPDATE ON "user"
FOR EACH ROW EXECUTE FUNCTION update_user_timestamps();
