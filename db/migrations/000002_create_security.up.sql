CREATE TYPE permission AS ENUM ('manage_user', 'view_user');

CREATE TABLE "role" (
    id uuid PRIMARY KEY,
    permissions permission[] NOT NULL DEFAULT ARRAY[]::permission[],
    CONSTRAINT role_id_not_nil CHECK (id <> '00000000-0000-0000-0000-000000000000'::uuid),
    CONSTRAINT role_permissions_valid CHECK (
        (cardinality(permissions) = 0 OR array_ndims(permissions) = 1)
        AND array_position(permissions, NULL::permission) IS NULL
    )
);

CREATE TABLE "user" (
    id uuid PRIMARY KEY,
    role_id uuid NOT NULL REFERENCES "role" (id) ON DELETE RESTRICT,
    email text NOT NULL,
    phone varchar(16) NOT NULL,
    password_hash text NOT NULL,
    first_name varchar(100) NOT NULL,
    last_name varchar(100) NOT NULL,
    CONSTRAINT user_email_not_blank CHECK (email ~ '[^[:space:]]'),
    CONSTRAINT user_phone_format CHECK (phone ~ '^[+][1-9][0-9]{1,14}$'),
    CONSTRAINT user_password_hash_not_blank CHECK (password_hash ~ '[^[:space:]]'),
    CONSTRAINT user_first_name_not_blank CHECK (first_name ~ '[^[:space:]]'),
    CONSTRAINT user_last_name_not_blank CHECK (last_name ~ '[^[:space:]]')
);

CREATE INDEX user_role_id_idx ON "user" (role_id);
