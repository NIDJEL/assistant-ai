ALTER TABLE dialogue_exchanges ADD COLUMN notification BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE dialogue_exchanges ADD COLUMN read_at TIMESTAMPTZ;
CREATE INDEX dialogue_notifications ON dialogue_exchanges(user_id,id DESC) WHERE notification;
CREATE INDEX zoom_remote_links ON cloud_event_links(remote_id) WHERE provider='zoom';
CREATE TABLE zoom_meeting_runs (
 user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 event_id BIGINT NOT NULL,
 meeting_id TEXT NOT NULL,
 run_uuid TEXT NOT NULL,
 started_at TIMESTAMPTZ,
 ended_at TIMESTAMPTZ,
 notified BOOLEAN NOT NULL DEFAULT false,
 PRIMARY KEY(user_id,meeting_id,run_uuid)
);
CREATE INDEX zoom_event_runs ON zoom_meeting_runs(user_id,event_id,started_at DESC);
