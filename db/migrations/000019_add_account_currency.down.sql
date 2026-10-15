-- Dropping the column also drops its foreign key and index. The accounts
-- deleted by the up migration are not restored.
ALTER TABLE "account" DROP COLUMN currency_code;
