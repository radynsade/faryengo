CREATE TABLE "currency" (
    code varchar(3) PRIMARY KEY,
    symbol varchar(5) NOT NULL,
    CONSTRAINT currency_code_format CHECK (code ~ '^[A-Z]{3}$'),
    -- The application validates which characters a symbol may contain; the
    -- database only rejects blank symbols and symbols with whitespace.
    CONSTRAINT currency_symbol_format CHECK (symbol ~ '^[^[:space:]]+$')
);
