CREATE TABLE "category" (
    id uuid PRIMARY KEY,
    workspace_id uuid NOT NULL,
    name varchar(100) NOT NULL,
    icon varchar(32),
    color varchar(16) NOT NULL,
    CONSTRAINT category_id_not_nil CHECK (id <> '00000000-0000-0000-0000-000000000000'::uuid),
    CONSTRAINT category_name_not_blank CHECK (name ~ '[^[:space:]]'),
    CONSTRAINT category_workspace_id_fkey FOREIGN KEY (workspace_id)
        REFERENCES "workspace" (id) ON DELETE CASCADE,
    CONSTRAINT category_color_valid CHECK (color IN (
        'green',
        'blue',
        'orange',
        'red',
        'purple',
        'brown',
        'grey',
        'yellow'
    )),
    -- The application validates icon names against its catalog, so adding an
    -- icon needs no migration; the database only rejects malformed names.
    CONSTRAINT category_icon_format CHECK (icon IS NULL OR icon ~ '^[a-z0-9-]+$')
);

-- This index serves listing a workspace's categories and the cascade when a
-- workspace is deleted.
CREATE INDEX category_workspace_id_idx ON "category" (workspace_id);
