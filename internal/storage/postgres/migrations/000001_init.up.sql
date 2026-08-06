BEGIN;

CREATE TABLE IF NOT EXISTS deployment_events (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source            TEXT        NOT NULL,
    pipeline_name     TEXT        NOT NULL DEFAULT '',
    pipeline_id       TEXT        NOT NULL,
    service_name      TEXT        NOT NULL,
    environment       TEXT        NOT NULL,
    status            TEXT        NOT NULL,
    started_at        TIMESTAMPTZ NOT NULL,
    finished_at       TIMESTAMPTZ,
    commit_sha        TEXT        NOT NULL DEFAULT '',
    commit_timestamp  TIMESTAMPTZ,
    metadata          JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT deployment_events_source_check
        CHECK (source IN ('jenkins','github-actions','gitlab-ci')),
    CONSTRAINT deployment_events_status_check
        CHECK (status IN ('in_progress','success','failure','cancelled')),
    CONSTRAINT deployment_events_finished_after_started
        CHECK (finished_at IS NULL OR finished_at >= started_at)
);

-- Dedup key: repeated webhook posts for the same pipeline run land on the
-- same row and update it, rather than duplicating.
CREATE UNIQUE INDEX IF NOT EXISTS deployment_events_dedup_idx
    ON deployment_events (source, pipeline_id);

-- Hot path for DORA queries filtered by service/environment.
CREATE INDEX IF NOT EXISTS deployment_events_service_env_finished_idx
    ON deployment_events (service_name, environment, finished_at DESC);

-- Global recent-events view (dashboards, admin UI).
CREATE INDEX IF NOT EXISTS deployment_events_finished_desc_idx
    ON deployment_events (finished_at DESC)
    WHERE finished_at IS NOT NULL;

CREATE OR REPLACE FUNCTION deployment_events_touch_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER deployment_events_touch_updated_at_trg
BEFORE UPDATE ON deployment_events
FOR EACH ROW EXECUTE FUNCTION deployment_events_touch_updated_at();

COMMIT;
