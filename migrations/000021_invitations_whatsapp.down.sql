DROP TABLE identity_verifications;

DROP TABLE invitations;

DROP INDEX signup_verifications_phone_key;
ALTER TABLE signup_verifications DROP CONSTRAINT signup_verifications_channel_identifier;
ALTER TABLE signup_verifications DROP COLUMN channel;
ALTER TABLE signup_verifications DROP COLUMN phone;
ALTER TABLE signup_verifications ALTER COLUMN email SET NOT NULL;

ALTER TABLE users DROP CONSTRAINT users_email_or_phone_required;
ALTER TABLE users ALTER COLUMN email SET NOT NULL;
