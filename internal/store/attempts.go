package store

import (
	"database/sql"
	"errors"
)

// ReserveAttempt 在验证凭证之前调用，原子地占用一次尝试名额。
// 窗口内前 max 次返回 true；第 max 次占用后锁定 lockFor 秒，锁定期内返回 false 且不改动记录。
// 锁定过期或距上次尝试超过 window 秒，则重新从 1 计数。凭证验证成功后应调用 ClearAttempts。
func (s *Store) ReserveAttempt(key string, now, window int64, max int, lockFor int64) (bool, error) {
	allowed := false
	err := s.inTx(func(tx *sql.Tx) error {
		var count, lockedUntil, lastAttempt int64
		err := tx.QueryRow(
			`SELECT failed_count, locked_until, last_attempt_at FROM login_attempts WHERE key = ?`, key,
		).Scan(&count, &lockedUntil, &lastAttempt)
		exists := err == nil
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if exists && lockedUntil > now {
			return nil
		}
		if !exists || lockedUntil != 0 || lastAttempt < now-window {
			count = 0
		}
		count++
		lockedUntil = 0
		if count >= int64(max) {
			lockedUntil = now + lockFor
		}
		_, err = tx.Exec(`
INSERT INTO login_attempts (key, failed_count, locked_until, last_attempt_at) VALUES (?, ?, ?, ?)
ON CONFLICT(key) DO UPDATE SET failed_count = excluded.failed_count,
    locked_until = excluded.locked_until, last_attempt_at = excluded.last_attempt_at`,
			key, count, lockedUntil, now,
		)
		allowed = err == nil
		return err
	})
	return allowed, err
}

func (s *Store) ClearAttempts(key string) error {
	_, err := s.db.Exec(`DELETE FROM login_attempts WHERE key = ?`, key)
	return err
}

// PurgeAttempts 删除窗口外且未锁定的记录，防止用随机地址刷出大量 key。
func (s *Store) PurgeAttempts(now, window int64) error {
	_, err := s.db.Exec(`DELETE FROM login_attempts WHERE locked_until <= ? AND last_attempt_at < ?`, now, now-window)
	return err
}
