DROP TRIGGER protect_fallback_language ON "language";

DROP FUNCTION protect_fallback_language();

DROP INDEX language_is_fallback_true;

ALTER TABLE "language" DROP COLUMN is_fallback;
