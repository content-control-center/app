-- secret stores envelope-encrypted API keys (CON-64). The KEK lives on
-- disk; each row carries a per-row DEK wrapped by the KEK. algorithm
-- and kek_version are forward-compat seams for future rotation.
CREATE TABLE IF NOT EXISTS secret (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    name         TEXT     NOT NULL UNIQUE,
    ciphertext   BLOB     NOT NULL,
    nonce        BLOB     NOT NULL,
    wrapped_dek  BLOB     NOT NULL,
    dek_nonce    BLOB     NOT NULL,
    algorithm    TEXT     NOT NULL DEFAULT 'AES-256-GCM',
    kek_version  INTEGER  NOT NULL DEFAULT 1,
    created_at   DATETIME NOT NULL,
    updated_at   DATETIME NOT NULL
);
