ALTER TABLE oauth_states ADD COLUMN source TEXT NOT NULL DEFAULT '' CHECK(source IN ('','onboarding','integrations'));

CREATE TABLE push_subscriptions (
 id UUID PRIMARY KEY, user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 endpoint TEXT NOT NULL UNIQUE, p256dh TEXT NOT NULL, auth TEXT NOT NULL,
 user_agent TEXT NOT NULL DEFAULT '', platform TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL,
 last_success_at TIMESTAMPTZ, last_failure_at TIMESTAMPTZ,
 failure_count INTEGER NOT NULL DEFAULT 0, disabled_at TIMESTAMPTZ,
 UNIQUE(id,user_id)
);
CREATE INDEX push_subscriptions_active ON push_subscriptions(user_id) WHERE disabled_at IS NULL;

CREATE TABLE notification_occurrences (
 id UUID PRIMARY KEY, user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 event_id BIGINT REFERENCES events(id) ON DELETE CASCADE, occurrence_at TIMESTAMPTZ NOT NULL,
 created_at TIMESTAMPTZ NOT NULL, expires_at TIMESTAMPTZ NOT NULL,
 payload JSONB NOT NULL, event_snapshot JSONB,
 UNIQUE(event_id,occurrence_at), UNIQUE(id,user_id)
);
CREATE INDEX notification_occurrences_user ON notification_occurrences(user_id,created_at);

CREATE TABLE notification_deliveries (
 id UUID PRIMARY KEY, notification_id UUID NOT NULL, subscription_id UUID NOT NULL,
 user_id UUID NOT NULL, channel TEXT NOT NULL DEFAULT 'web_push' CHECK(channel='web_push'),
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','retry','success','dead','failed','expired','cancelled')),
 attempts INTEGER NOT NULL DEFAULT 0, next_attempt_at TIMESTAMPTZ NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL, http_status INTEGER NOT NULL DEFAULT 0,
 FOREIGN KEY(notification_id,user_id) REFERENCES notification_occurrences(id,user_id) ON DELETE CASCADE,
 FOREIGN KEY(subscription_id,user_id) REFERENCES push_subscriptions(id,user_id) ON DELETE CASCADE,
 UNIQUE(notification_id,subscription_id)
);
CREATE INDEX notification_deliveries_pending ON notification_deliveries(next_attempt_at) WHERE status IN ('pending','retry');
CREATE TABLE push_test_limits (user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE, last_sent_at TIMESTAMPTZ NOT NULL);
