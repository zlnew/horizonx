-- P1-11: env var values are encrypted at rest; this flag distinguishes
-- ciphertext rows from legacy plaintext rows written before encryption.
ALTER TABLE environment_variables ADD COLUMN IF NOT EXISTS value_encrypted BOOLEAN NOT NULL DEFAULT FALSE;
