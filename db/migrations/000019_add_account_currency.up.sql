-- Every account must hold its money in a currency. There is no production data
-- yet, so existing accounts are removed instead of being assigned a currency.
DELETE FROM "account";

ALTER TABLE "account"
    ADD COLUMN currency_code varchar(3) NOT NULL,
    -- A currency cannot be deleted while an account uses it.
    ADD CONSTRAINT account_currency_code_fkey FOREIGN KEY (currency_code)
        REFERENCES "currency" (code) ON DELETE RESTRICT;

-- This index serves the check that a deleted currency is unused.
CREATE INDEX account_currency_code_idx ON "account" (currency_code);
