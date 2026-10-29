CREATE TABLE accounts (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    address             TEXT    NOT NULL UNIQUE,
    password_hash       TEXT    NOT NULL,
    enabled             INTEGER NOT NULL DEFAULT 1,
    note                TEXT    NOT NULL DEFAULT '',
    created_at          INTEGER NOT NULL,
    password_updated_at INTEGER NOT NULL,
    last_login_at       INTEGER
);

CREATE TABLE sessions (
    token_hash TEXT    PRIMARY KEY,
    role       TEXT    NOT NULL CHECK (role IN ('admin', 'user')),
    account_id INTEGER REFERENCES accounts(id) ON DELETE CASCADE,
    expires_at INTEGER NOT NULL
);

CREATE INDEX idx_sessions_account ON sessions(account_id);
CREATE INDEX idx_sessions_expires ON sessions(expires_at);

CREATE TABLE login_attempts (
    key          TEXT    PRIMARY KEY,
    failed_count    INTEGER NOT NULL,
    locked_until    INTEGER NOT NULL DEFAULT 0,
    last_attempt_at INTEGER NOT NULL
);
