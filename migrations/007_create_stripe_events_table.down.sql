-- Drop stripe_events table
DROP INDEX IF EXISTS idx_stripe_events_event_type;
DROP INDEX IF EXISTS idx_stripe_events_event_id;
DROP TABLE IF EXISTS stripe_events;