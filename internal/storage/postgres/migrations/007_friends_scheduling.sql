-- Additive migration: existing accounts and events remain the source of truth.
ALTER TABLE users ADD COLUMN friend_code TEXT UNIQUE;
ALTER TABLE users ADD COLUMN share_friend_email BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN planning_timezone TEXT NOT NULL DEFAULT 'Europe/Moscow';
ALTER TABLE users ADD COLUMN planning_start_hour INTEGER NOT NULL DEFAULT 9 CHECK(planning_start_hour BETWEEN 0 AND 23);
ALTER TABLE users ADD COLUMN planning_end_hour INTEGER NOT NULL DEFAULT 21 CHECK(planning_end_hour BETWEEN 1 AND 24);
ALTER TABLE users ADD CONSTRAINT planning_hours CHECK(planning_start_hour < planning_end_hour);
CREATE FUNCTION assign_friend_code() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE candidate TEXT;
BEGIN
 IF NEW.friend_code IS NULL THEN
  PERFORM pg_advisory_xact_lock(73121013);
  LOOP
   candidate:=translate(substr(md5(gen_random_uuid()::text),1,8),'0123456789abcdef','ABCDEFGHJKLMNPQR');
   candidate:=substr(candidate,1,4)||'-'||substr(candidate,5,4);
   EXIT WHEN NOT EXISTS(SELECT 1 FROM users WHERE friend_code=candidate);
  END LOOP;
  NEW.friend_code:=candidate;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER user_friend_code BEFORE INSERT OR UPDATE OF friend_code ON users FOR EACH ROW EXECUTE FUNCTION assign_friend_code();
UPDATE users SET friend_code=NULL WHERE friend_code IS NULL;

CREATE TABLE friend_requests (
 id UUID PRIMARY KEY, sender_id UUID NOT NULL REFERENCES users(id), recipient_id UUID NOT NULL REFERENCES users(id),
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','accepted','declined','cancelled')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), responded_at TIMESTAMPTZ,
 CHECK(sender_id<>recipient_id)
);
CREATE UNIQUE INDEX friend_request_pending_pair ON friend_requests(least(sender_id,recipient_id),greatest(sender_id,recipient_id)) WHERE status='pending';
CREATE INDEX friend_request_inbox ON friend_requests(recipient_id,status);
CREATE TABLE friendships (
 user_low UUID NOT NULL REFERENCES users(id), user_high UUID NOT NULL REFERENCES users(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY(user_low,user_high), CHECK(user_low<user_high)
);
CREATE TABLE friend_tags (
 user_id UUID NOT NULL REFERENCES users(id), friend_id UUID NOT NULL REFERENCES users(id),
 tags JSONB NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(tags)='array'), PRIMARY KEY(user_id,friend_id), CHECK(user_id<>friend_id)
);
CREATE TABLE social_rate_limits (
 user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE, bucket TEXT NOT NULL,
 window_start TIMESTAMPTZ NOT NULL, attempts INTEGER NOT NULL DEFAULT 1, PRIMARY KEY(user_id,bucket)
);
CREATE TABLE collaborative_meetings (
 event_id BIGINT PRIMARY KEY REFERENCES events(id), version INTEGER NOT NULL DEFAULT 1,
 cancelled BOOLEAN NOT NULL DEFAULT false, timezone TEXT NOT NULL
);
CREATE TABLE meeting_participants (
 event_id BIGINT NOT NULL REFERENCES collaborative_meetings(event_id), user_id UUID NOT NULL REFERENCES users(id),
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','accepted','declined','cancelled')),
 version INTEGER NOT NULL DEFAULT 1, response_revision INTEGER NOT NULL DEFAULT 0,
 invitation_generation INTEGER NOT NULL DEFAULT 1, cancelled_snapshot JSONB,
 invited_at TIMESTAMPTZ NOT NULL DEFAULT now(), responded_at TIMESTAMPTZ,
 PRIMARY KEY(event_id,user_id)
);
CREATE INDEX meeting_participant_calendar ON meeting_participants(user_id,status,event_id);
CREATE TABLE invitation_responses (
 id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY, event_id BIGINT NOT NULL, user_id UUID NOT NULL,
 version INTEGER NOT NULL, revision INTEGER NOT NULL, status TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(event_id,user_id,version,revision),
 FOREIGN KEY(event_id,user_id) REFERENCES meeting_participants(event_id,user_id)
);
CREATE TABLE reschedule_proposals (
 id UUID PRIMARY KEY, event_id BIGINT NOT NULL REFERENCES collaborative_meetings(event_id),
 proposer_id UUID NOT NULL REFERENCES users(id), version INTEGER NOT NULL,
 slots JSONB NOT NULL CHECK(jsonb_typeof(slots)='array'),
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','accepted','declined','superseded')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), resolved_at TIMESTAMPTZ
);
CREATE TABLE social_operations (
 user_id UUID NOT NULL REFERENCES users(id), request_id UUID NOT NULL, fingerprint TEXT NOT NULL,
 result JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY(user_id,request_id)
);
-- Only opaque intervals are cached. No titles, descriptions, attendees or tokens.
CREATE TABLE availability_snapshots (
 user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE, provider TEXT NOT NULL,
 starts_at TIMESTAMPTZ NOT NULL, ends_at TIMESTAMPTZ NOT NULL,
 busy JSONB NOT NULL DEFAULT '[]', checked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 complete BOOLEAN NOT NULL, PRIMARY KEY(user_id,provider)
);

-- Accepted attendees export the same event through the existing durable outboxes.
CREATE FUNCTION enqueue_shared_calendar() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE eid BIGINT; uid UUID;
BEGIN
 IF TG_TABLE_NAME='events' THEN eid:=NEW.id; ELSE eid:=NEW.event_id; END IF;
 FOR uid IN SELECT user_id FROM meeting_participants WHERE event_id=eid LOOP
  INSERT INTO calendar_outbox(user_id,event_id)
   SELECT uid,eid WHERE EXISTS(SELECT 1 FROM calendar_connections WHERE user_id=uid AND status<>'disconnected')
   ON CONFLICT(user_id,event_id) DO UPDATE SET version=calendar_outbox.version+1;
  INSERT INTO cloud_outbox(user_id,provider,event_id)
   SELECT uid,'google',eid WHERE EXISTS(SELECT 1 FROM cloud_connections WHERE user_id=uid AND provider='google' AND status<>'disconnected')
   ON CONFLICT(user_id,provider,event_id) DO UPDATE SET version=cloud_outbox.version+1;
 END LOOP;
 RETURN NULL;
END $$;
CREATE TRIGGER shared_calendar_event AFTER UPDATE ON events FOR EACH ROW EXECUTE FUNCTION enqueue_shared_calendar();
CREATE TRIGGER shared_calendar_response AFTER INSERT OR UPDATE ON meeting_participants FOR EACH ROW EXECUTE FUNCTION enqueue_shared_calendar();
