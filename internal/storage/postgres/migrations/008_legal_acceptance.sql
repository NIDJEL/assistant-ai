CREATE TABLE user_legal_acceptances (
    id bigserial PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    document_version text NOT NULL,
    terms_accepted boolean NOT NULL DEFAULT true CHECK (terms_accepted),
    personal_data_consent boolean NOT NULL DEFAULT true CHECK (personal_data_consent),
    accepted_at timestamptz NOT NULL,
    action text NOT NULL CHECK (action IN ('email_register', 'email_login', 'yandex_login'))
);
CREATE INDEX user_legal_acceptances_user_idx ON user_legal_acceptances(user_id, accepted_at);

-- Bind the explicit confirmation to the same one-use OAuth state and browser.
-- Existing sessions and users are deliberately not assigned consent records.
ALTER TABLE oauth_states ADD COLUMN legal_acceptance jsonb;
