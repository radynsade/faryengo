DROP TRIGGER update_user_timestamps ON "user";
DROP FUNCTION update_user_timestamps();

ALTER TABLE "user"
    DROP COLUMN email_changed_at,
    DROP COLUMN phone_changed_at,
    DROP COLUMN password_changed_at,
    DROP COLUMN updated_at,
    DROP COLUMN created_at;
