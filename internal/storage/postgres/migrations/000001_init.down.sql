BEGIN;

DROP TRIGGER IF EXISTS deployment_events_touch_updated_at_trg ON deployment_events;
DROP FUNCTION IF EXISTS deployment_events_touch_updated_at();
DROP TABLE IF EXISTS deployment_events;

COMMIT;
