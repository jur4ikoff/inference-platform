-- BEGIN;

-- ALTER TABLE users RENAME COLUMN login TO email;
-- ALTER TABLE users RENAME COLUMN hashed_password TO password_hash;
-- ALTER INDEX users_login_lower_unique_idx RENAME TO users_email_lower_unique_idx;

-- UPDATE users SET email = LOWER(TRIM(email));

-- ALTER TABLE users
--     ADD COLUMN role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
--     ALTER COLUMN firstname SET DEFAULT '',
--     ALTER COLUMN lastname SET DEFAULT '';

-- COMMIT;
