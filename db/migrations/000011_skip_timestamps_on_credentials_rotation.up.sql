-- A row change that touches only credentials_version is a revocation, not an
-- account change, so it keeps every timestamp. Otherwise signing out
-- everywhere would advance updated_at and fail concurrent account updates that
-- use it as their concurrency token. Triggers run in name order, so
-- rotate_user_credentials_version has already rotated the version when the
-- email or password changed; those updates still differ in other columns.
CREATE OR REPLACE FUNCTION update_user_timestamps() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    excluded text[] := ARRAY[
        'credentials_version',
        'email_changed_at',
        'phone_changed_at',
        'password_changed_at',
        'updated_at',
        'created_at'
    ];
BEGIN
    NEW.created_at := OLD.created_at;
    NEW.email_changed_at := OLD.email_changed_at;
    NEW.phone_changed_at := OLD.phone_changed_at;
    NEW.password_changed_at := OLD.password_changed_at;

    IF NEW.credentials_version IS DISTINCT FROM OLD.credentials_version
       AND to_jsonb(NEW) - excluded = to_jsonb(OLD) - excluded THEN
        NEW.updated_at := OLD.updated_at;
        RETURN NEW;
    END IF;

    NEW.updated_at := GREATEST(clock_timestamp(), OLD.updated_at + interval '1 microsecond');

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
