-- PostgreSQL cannot remove enum values directly. The conversion below fails
-- if any role still uses manage_role or view_role, preserving its permissions
-- when the migration transaction rolls back.
ALTER TABLE "role" DROP CONSTRAINT role_permissions_valid;
ALTER TABLE "role" ALTER COLUMN permissions DROP DEFAULT;

CREATE TYPE permission_previous AS ENUM ('manage_user', 'view_user');

ALTER TABLE "role"
    ALTER COLUMN permissions TYPE permission_previous[]
    USING permissions::text[]::permission_previous[];

DROP TYPE permission;
ALTER TYPE permission_previous RENAME TO permission;

ALTER TABLE "role"
    ALTER COLUMN permissions SET DEFAULT ARRAY[]::permission[],
    ADD CONSTRAINT role_permissions_valid CHECK (
        (cardinality(permissions) = 0 OR array_ndims(permissions) = 1)
        AND array_position(permissions, NULL::permission) IS NULL
    );
