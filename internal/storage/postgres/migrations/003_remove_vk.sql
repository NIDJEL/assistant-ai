-- Remove the retired provider only. Accounts and their personal data survive.
DELETE FROM oauth_states WHERE provider = 'vk';
DELETE FROM auth_identities WHERE provider = 'vk';
ALTER TABLE auth_identities DROP CONSTRAINT auth_identities_provider_check;
ALTER TABLE auth_identities ADD CONSTRAINT auth_identities_provider_check
 CHECK (provider IN ('email', 'yandex'));
