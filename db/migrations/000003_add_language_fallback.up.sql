ALTER TABLE "language" ADD COLUMN is_fallback boolean NOT NULL DEFAULT false;

CREATE UNIQUE INDEX language_is_fallback_true ON "language" (is_fallback) WHERE is_fallback;

CREATE FUNCTION protect_fallback_language() RETURNS trigger AS $$
DECLARE
    removing_fallback boolean := TG_OP = 'DELETE';
BEGIN
    IF TG_OP = 'UPDATE' THEN
        removing_fallback := NOT NEW.is_fallback;
    END IF;

    IF OLD.is_fallback AND removing_fallback THEN
        -- Block concurrent translation inserts while checking usage.
        PERFORM 1 FROM "language" WHERE code = OLD.code FOR UPDATE;

        IF EXISTS (SELECT 1 FROM "translation" WHERE language_code = OLD.code) THEN
            RAISE EXCEPTION USING
                ERRCODE = '23514',
                CONSTRAINT = 'language_fallback_in_use',
                MESSAGE = 'The fallback language with existing translations cannot be deleted or unset.';
        END IF;
    END IF;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER protect_fallback_language
BEFORE DELETE OR UPDATE OF is_fallback ON "language"
FOR EACH ROW EXECUTE FUNCTION protect_fallback_language();
