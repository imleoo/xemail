CREATE TABLE emails (
    id          TEXT    PRIMARY KEY,
    from_addr   TEXT    NOT NULL,
    to_addrs    TEXT    NOT NULL,
    cc_addrs    TEXT    NOT NULL,
    subject     TEXT    NOT NULL,
    html        TEXT    NOT NULL,
    text        TEXT    NOT NULL,
    message_id  TEXT    NOT NULL DEFAULT '',
    received_at INTEGER NOT NULL
);

CREATE INDEX idx_emails_received ON emails(received_at DESC);

-- 收件人归一化地址（received_for ∪ to ∪ cc ∪ bcc）。
-- 邮件归属于哪个账号在查询时按 accounts.address 动态匹配，
-- 因此账号晚于邮件创建时，历史邮件也会自动出现在该账号的收件箱。
CREATE TABLE email_recipients (
    email_id TEXT NOT NULL REFERENCES emails(id) ON DELETE CASCADE,
    address  TEXT NOT NULL,
    PRIMARY KEY (email_id, address)
);

CREATE INDEX idx_recipients_address ON email_recipients(address);

CREATE TABLE attachments (
    id           TEXT NOT NULL,
    email_id     TEXT NOT NULL REFERENCES emails(id) ON DELETE CASCADE,
    filename     TEXT NOT NULL,
    content_type TEXT NOT NULL,
    PRIMARY KEY (email_id, id)
);

CREATE TABLE mail_reads (
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    email_id   TEXT    NOT NULL REFERENCES emails(id) ON DELETE CASCADE,
    PRIMARY KEY (account_id, email_id)
);
