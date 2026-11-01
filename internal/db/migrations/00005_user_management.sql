-- +goose Up
ALTER TABLE users ADD COLUMN must_change_password boolean NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN temporary_password_expires_at timestamptz;
ALTER TABLE users ADD CONSTRAINT temporary_password_expiry CHECK
    (must_change_password = (temporary_password_expires_at IS NOT NULL));
