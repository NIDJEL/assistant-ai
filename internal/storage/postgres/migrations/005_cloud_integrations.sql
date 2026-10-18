CREATE TABLE cloud_connections (
 user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 provider TEXT NOT NULL CHECK(provider IN ('google','zoom')),
 subject TEXT NOT NULL, login TEXT NOT NULL DEFAULT '',
 token_ciphertext BYTEA, expires_at TIMESTAMPTZ,
 status TEXT NOT NULL DEFAULT 'connected', last_error TEXT NOT NULL DEFAULT '',
 calendar_id TEXT NOT NULL DEFAULT '', sync_token TEXT NOT NULL DEFAULT '',
 last_sync TIMESTAMPTZ, PRIMARY KEY(user_id,provider)
);
CREATE TABLE cloud_event_links (
 user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 provider TEXT NOT NULL, calendar_id TEXT NOT NULL DEFAULT '',
 event_id BIGINT NOT NULL, remote_id TEXT NOT NULL,
 etag TEXT NOT NULL DEFAULT '', remote_updated TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(user_id,provider,calendar_id,event_id),
 UNIQUE(user_id,provider,calendar_id,remote_id)
);
CREATE TABLE cloud_outbox (
 user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 provider TEXT NOT NULL, event_id BIGINT NOT NULL,
 version BIGINT NOT NULL DEFAULT 1, PRIMARY KEY(user_id,provider,event_id)
);
CREATE FUNCTION enqueue_cloud_event() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE uid UUID; eid BIGINT; p TEXT;
BEGIN
 IF current_setting('gena.cloud_import',true)='google' THEN RETURN NULL; END IF;
 IF TG_OP='DELETE' THEN uid:=OLD.user_id; eid:=OLD.id;
 ELSE uid:=NEW.user_id; eid:=NEW.id; END IF;
 FOR p IN SELECT provider FROM cloud_connections WHERE user_id=uid AND status<>'disconnected' LOOP
  IF p='google' OR EXISTS(SELECT 1 FROM cloud_event_links WHERE user_id=uid AND provider=p AND event_id=eid)
     OR (TG_OP<>'DELETE' AND NEW.details::jsonb->>'conference_provider'='zoom') THEN
   INSERT INTO cloud_outbox(user_id,provider,event_id) VALUES(uid,p,eid)
   ON CONFLICT(user_id,provider,event_id) DO UPDATE SET version=cloud_outbox.version+1;
  END IF;
 END LOOP;
 RETURN NULL;
END $$;
CREATE TRIGGER cloud_event_change AFTER INSERT OR UPDATE OR DELETE ON events
 FOR EACH ROW EXECUTE FUNCTION enqueue_cloud_event();
