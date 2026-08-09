-- Forced first-login password changes are disabled during the development stage.
-- Keep the column for a future configurable production password policy.
UPDATE occccad.users
SET must_change_password = false,
    updated_at = now()
WHERE must_change_password = true;
