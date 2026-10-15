-- A budget limits its amount during a period: a window, which may repeat. The
-- window's bounds are kept as instants, and the time zone keeps the calendar
-- that repeated windows follow. A budget without bounds and without a repeat
-- limits its amount at any time, which existing budgets keep doing.
ALTER TABLE "budget"
    ADD COLUMN period_from timestamptz,
    ADD COLUMN period_to timestamptz,
    ADD COLUMN period_repeat varchar(16) NOT NULL DEFAULT '',
    ADD COLUMN period_every interval,
    ADD COLUMN period_until timestamptz,
    ADD COLUMN period_time_zone varchar(64) NOT NULL DEFAULT 'UTC',
    ADD CONSTRAINT budget_period_ends_after_start CHECK (
        period_from IS NULL OR period_to IS NULL OR period_to > period_from
    ),
    ADD CONSTRAINT budget_period_repeat_valid CHECK (
        period_repeat IN ('', 'daily', 'weekly', 'monthly', 'yearly', 'every')
    ),
    -- Only a repeat of its own interval has a step, and that step is positive.
    ADD CONSTRAINT budget_period_every_valid CHECK (
        (period_repeat = 'every') = (period_every IS NOT NULL)
        AND (period_every IS NULL OR period_every > interval '0')
    ),
    -- A repeating window has both bounds, and only a repeat has an end.
    ADD CONSTRAINT budget_period_repeat_bounded CHECK (
        period_repeat = '' OR (period_from IS NOT NULL AND period_to IS NOT NULL)
    ),
    ADD CONSTRAINT budget_period_until_valid CHECK (
        period_until IS NULL OR (period_repeat <> '' AND period_until > period_from)
    ),
    ADD CONSTRAINT budget_period_time_zone_not_blank CHECK (period_time_zone ~ '[^[:space:]]');
