-- 已处理的 Resend Webhook 事件，按 svix-id 去重，防止重放重复触发同步。
CREATE TABLE webhook_events (
    svix_id     TEXT    PRIMARY KEY,
    type        TEXT    NOT NULL,
    received_at INTEGER NOT NULL
);
