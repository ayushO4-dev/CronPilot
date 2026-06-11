-- TOTP two-factor authentication: per-user secret + enabled flag. The secret
-- is stored from setup time but 2FA is only enforced once totp_enabled is set
-- (after the user has confirmed a valid code).

ALTER TABLE users ADD COLUMN totp_secret TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN totp_enabled INTEGER NOT NULL DEFAULT 0;
