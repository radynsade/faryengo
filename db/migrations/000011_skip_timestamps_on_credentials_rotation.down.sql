CREATE OR REPLACE FUNCTION update_user_timestamps() RETURNS trigger LANGUAGE plpgsql AS $$
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
