package store

// RecordWebhookEvent 记录一个已处理的 Webhook 事件；该 svix-id 已存在时返回 false。
func (s *Store) RecordWebhookEvent(svixID, eventType string, now int64) (bool, error) {
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO webhook_events (svix_id, type, received_at) VALUES (?, ?, ?)`,
		svixID, eventType, now,
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
