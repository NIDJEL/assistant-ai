ALTER TABLE oauth_states ADD COLUMN purpose TEXT NOT NULL DEFAULT '';
ALTER TABLE calendar_connections
 ADD COLUMN provider_subject TEXT NOT NULL DEFAULT '',
 ADD COLUMN login TEXT NOT NULL DEFAULT '',
 ADD COLUMN token_ciphertext BYTEA,
 ADD COLUMN expires_at TIMESTAMPTZ,
 ADD COLUMN calendar_url TEXT NOT NULL DEFAULT '',
 ADD COLUMN last_sync TIMESTAMPTZ,
 ADD COLUMN last_error TEXT NOT NULL DEFAULT '';

CREATE TABLE calendar_outbox (
 user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 event_id BIGINT NOT NULL,
 version BIGINT NOT NULL DEFAULT 1,
 PRIMARY KEY(user_id,event_id)
);

-- Every event writer (including chat, CSV imports and restores) shares this
-- transactional outbox. A failed remote request never loses a local change.
CREATE FUNCTION enqueue_calendar_event() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE uid UUID; eid BIGINT;
BEGIN
 IF TG_OP='DELETE' THEN uid:=OLD.user_id; eid:=OLD.id;
 ELSE uid:=NEW.user_id; eid:=NEW.id;
 END IF;
 IF EXISTS(SELECT 1 FROM calendar_connections WHERE user_id=uid AND provider='yandex' AND status<>'disconnected') THEN
  INSERT INTO calendar_outbox(user_id,event_id) VALUES(uid,eid)
  ON CONFLICT(user_id,event_id) DO UPDATE SET version=calendar_outbox.version+1;
 END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER calendar_event_change AFTER INSERT OR UPDATE OR DELETE ON events
 FOR EACH ROW EXECUTE FUNCTION enqueue_calendar_event();
