package store

import (
	"database/sql"
	"errors"
)

type Session struct {
	TokenHash string
	Role      string
	AccountID *int64
	ExpiresAt int64
}

func (s *Store) CreateSession(sess Session) error {
	_, err := s.db.Exec(
		`INSERT INTO sessions (token_hash, role, account_id, expires_at) VALUES (?, ?, ?, ?)`,
		sess.TokenHash, sess.Role, sess.AccountID, sess.ExpiresAt,
	)
	return err
}

// CreateUserSession 在同一事务内确认账号仍启用且密码哈希仍是 expectedHash 后再插入会话。
// 否则返回 ErrStale（账号不存在返回 ErrNotFound），防止重置/禁用与登录交错时漏出有效会话。
func (s *Store) CreateUserSession(accountID int64, expectedHash string, sess Session) error {
	return s.inTx(func(tx *sql.Tx) error {
		var hash string
		var enabled int
		err := tx.QueryRow(`SELECT password_hash, enabled FROM accounts WHERE id = ?`, accountID).Scan(&hash, &enabled)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if enabled != 1 || hash != expectedHash {
			return ErrStale
		}
		_, err = tx.Exec(
			`INSERT INTO sessions (token_hash, role, account_id, expires_at) VALUES (?, ?, ?, ?)`,
			sess.TokenHash, sess.Role, sess.AccountID, sess.ExpiresAt,
		)
		return err
	})
}

func (s *Store) GetSession(tokenHash string) (Session, error) {
	var sess Session
	err := s.db.QueryRow(
		`SELECT token_hash, role, account_id, expires_at FROM sessions WHERE token_hash = ?`, tokenHash,
	).Scan(&sess.TokenHash, &sess.Role, &sess.AccountID, &sess.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	return sess, err
}

func (s *Store) DeleteSession(tokenHash string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

func (s *Store) DeleteExpiredSessions(now int64) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, now)
	return err
}
