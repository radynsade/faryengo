CREATE TABLE "language" (
    code varchar(2) PRIMARY KEY,
    english_name varchar(100) NOT NULL,
    native_name varchar(100) NOT NULL,
    CONSTRAINT language_code_format CHECK (code ~ '^[a-z]{2}$'),
    CONSTRAINT language_english_name_not_blank CHECK (english_name ~ '[^[:space:]]'),
    CONSTRAINT language_native_name_not_blank CHECK (native_name ~ '[^[:space:]]')
);

CREATE TABLE "text" (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY
);

CREATE TABLE "translation" (
    text_id bigint NOT NULL REFERENCES "text" (id) ON DELETE CASCADE,
    language_code varchar(2) NOT NULL REFERENCES "language" (code) ON DELETE RESTRICT,
    content text NOT NULL,
    CONSTRAINT translation_pkey PRIMARY KEY (text_id, language_code),
    CONSTRAINT translation_content_not_blank CHECK (content ~ '[^[:space:]]')
);

CREATE INDEX translation_language_code_idx ON "translation" (language_code);
