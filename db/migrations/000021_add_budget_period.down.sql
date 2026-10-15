-- Dropping the columns also drops their constraints.
ALTER TABLE "budget"
    DROP COLUMN period_from,
    DROP COLUMN period_to,
    DROP COLUMN period_repeat,
    DROP COLUMN period_every,
    DROP COLUMN period_until,
    DROP COLUMN period_time_zone;
