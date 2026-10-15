-- A budget may cover only categories of its own workspace. The composite
-- foreign keys below enforce that, and they need these keys to reference.
ALTER TABLE "category"
    ADD CONSTRAINT category_id_workspace_id_key UNIQUE (id, workspace_id);

CREATE TABLE "budget" (
    id uuid PRIMARY KEY,
    workspace_id uuid NOT NULL,
    currency_code varchar(3) NOT NULL,
    -- The amount is counted in the minor units of the currency.
    amount bigint NOT NULL,
    CONSTRAINT budget_id_not_nil CHECK (id <> '00000000-0000-0000-0000-000000000000'::uuid),
    CONSTRAINT budget_amount_not_negative CHECK (amount >= 0),
    CONSTRAINT budget_id_workspace_id_key UNIQUE (id, workspace_id),
    CONSTRAINT budget_workspace_id_fkey FOREIGN KEY (workspace_id)
        REFERENCES "workspace" (id) ON DELETE CASCADE,
    -- A currency cannot be deleted while a budget uses it.
    CONSTRAINT budget_currency_code_fkey FOREIGN KEY (currency_code)
        REFERENCES "currency" (code) ON DELETE RESTRICT
);

-- These indexes serve listing a workspace's budgets, the cascade when a
-- workspace is deleted, and the check that a deleted currency is unused.
CREATE INDEX budget_workspace_id_idx ON "budget" (workspace_id);
CREATE INDEX budget_currency_code_idx ON "budget" (currency_code);

CREATE TABLE "budget_category" (
    budget_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    category_id uuid NOT NULL,
    -- The position keeps the categories in the order they were written.
    position integer NOT NULL,
    CONSTRAINT budget_category_pkey PRIMARY KEY (budget_id, category_id),
    CONSTRAINT budget_category_position_key UNIQUE (budget_id, position),
    CONSTRAINT budget_category_position_not_negative CHECK (position >= 0),
    CONSTRAINT budget_category_budget_fkey FOREIGN KEY (budget_id, workspace_id)
        REFERENCES "budget" (id, workspace_id) ON DELETE CASCADE,
    -- A category cannot be deleted while a budget covers it. NO ACTION rather
    -- than RESTRICT defers the check to the end of the statement, so deleting
    -- a workspace cascades to its budgets before its categories are checked.
    CONSTRAINT budget_category_category_fkey FOREIGN KEY (category_id, workspace_id)
        REFERENCES "category" (id, workspace_id) ON DELETE NO ACTION
);

-- This index serves listing the budgets that cover a category and the check
-- that a deleted category is unused.
CREATE INDEX budget_category_category_id_idx ON "budget_category" (category_id, workspace_id);
