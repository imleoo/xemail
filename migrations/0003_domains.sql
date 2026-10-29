-- 收信域名白名单，由管理员在后台维护。
-- 表为空表示不限制账号所属域名。
CREATE TABLE domains (
    name       TEXT    PRIMARY KEY,
    created_at INTEGER NOT NULL
);
