-- Remove billing columns from users table
DROP INDEX IF EXISTS idx_users_stripe_customer_id;

ALTER TABLE users
    DROP COLUMN IF EXISTS current_period_end,
    DROP COLUMN IF EXISTS stripe_customer_id,
    DROP COLUMN IF EXISTS subscription_status,
    DROP COLUMN IF EXISTS plan;