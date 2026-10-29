package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

type Attachment struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
}

type MailSummary struct {
	ID             string   `json:"id"`
	From           string   `json:"from"`
	To             []string `json:"to"`
	Subject        string   `json:"subject"`
	ReceivedAt     int64    `json:"received_at"`
	IsRead         bool     `json:"is_read"`
	HasAttachments bool     `json:"has_attachments"`
}

type Mail struct {
	MailSummary
	Cc          []string     `json:"cc"`
	HTML        string       `json:"html"`
	Text        string       `json:"text"`
	Attachments []Attachment `json:"attachments"`
}

type NewMail struct {
	ID          string
	From        string
	To          []string
	Cc          []string
	Subject     string
	HTML        string
	Text        string
	MessageID   string
	ReceivedAt  int64
	Recipients  []string // 已归一化（小写）的全部收件地址
	Attachments []Attachment
}

// 邮件归属于某账号：任一收件地址等于该账号地址。
const matchAccount = `EXISTS (SELECT 1 FROM email_recipients r JOIN accounts a ON a.address = r.address WHERE r.email_id = e.id AND a.id = ?)`

// 未匹配：没有任何收件地址对应已存在的账号。
const matchNone = `NOT EXISTS (SELECT 1 FROM email_recipients r JOIN accounts a ON a.address = r.address WHERE r.email_id = e.id)`

// ExistingEmailIDs 返回 ids 中已入库的邮件 id 集合。
func (s *Store) ExistingEmailIDs(ids []string) (map[string]bool, error) {
	found := make(map[string]bool, len(ids))
	const chunk = 500
	for start := 0; start < len(ids); start += chunk {
		part := ids[start:min(start+chunk, len(ids))]
		args := make([]any, len(part))
		for i, id := range part {
			args[i] = id
		}
		rows, err := s.db.Query(`SELECT id FROM emails WHERE id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(part)), ",")+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			found[id] = true
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return found, nil
}

// InsertEmail 在一个事务内写入邮件、收件人与附件元数据；同 id 重复写入被忽略。
func (s *Store) InsertEmail(m NewMail) error {
	to, _ := json.Marshal(nonNil(m.To))
	cc, _ := json.Marshal(nonNil(m.Cc))
	return s.inTx(func(tx *sql.Tx) error {
		res, err := tx.Exec(
			`INSERT OR IGNORE INTO emails (id, from_addr, to_addrs, cc_addrs, subject, html, text, message_id, received_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			m.ID, m.From, string(to), string(cc), m.Subject, m.HTML, m.Text, m.MessageID, m.ReceivedAt,
		)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil
		}
		for _, addr := range m.Recipients {
			if _, err := tx.Exec(`INSERT OR IGNORE INTO email_recipients (email_id, address) VALUES (?, ?)`, m.ID, addr); err != nil {
				return err
			}
		}
		for _, a := range m.Attachments {
			if _, err := tx.Exec(
				`INSERT OR IGNORE INTO attachments (id, email_id, filename, content_type) VALUES (?, ?, ?, ?)`,
				a.ID, m.ID, a.Filename, a.ContentType,
			); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) ListAccountMails(accountID int64, limit, offset int) ([]MailSummary, int, error) {
	return s.listMails(matchAccount, accountID, accountID, limit, offset)
}

func (s *Store) ListUnroutedMails(limit, offset int) ([]MailSummary, int, error) {
	return s.listMails(matchNone, 0, 0, limit, offset)
}

// GetAccountMail 仅当邮件归属该账号时返回，否则 ErrNotFound。
func (s *Store) GetAccountMail(accountID int64, id string) (Mail, error) {
	return s.getMail(matchAccount, accountID, accountID, id)
}

// GetUnroutedMail 仅当邮件没有匹配任何账号时返回，管理员不能借此读取已分发给账号的邮件。
func (s *Store) GetUnroutedMail(id string) (Mail, error) {
	return s.getMail(matchNone, 0, 0, id)
}

func (s *Store) MarkRead(accountID int64, emailID string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO mail_reads (account_id, email_id) VALUES (?, ?)`, accountID, emailID)
	return err
}

// GetAttachment 返回邮件下的附件元数据。调用方须先通过 Get*Mail 校验邮件的访问权限。
func (s *Store) GetAttachment(emailID, attachmentID string) (Attachment, error) {
	var a Attachment
	err := s.db.QueryRow(
		`SELECT id, filename, content_type FROM attachments WHERE email_id = ? AND id = ?`, emailID, attachmentID,
	).Scan(&a.ID, &a.Filename, &a.ContentType)
	if errors.Is(err, sql.ErrNoRows) {
		return Attachment{}, ErrNotFound
	}
	return a, err
}

// listMails 的 cond 里至多含一个账号占位符；readAccountID 为 0 时 is_read 恒为 false。
func (s *Store) listMails(cond string, condArg, readAccountID int64, limit, offset int) ([]MailSummary, int, error) {
	args := func(extra ...any) []any {
		var out []any
		if strings.Contains(cond, "?") {
			out = append(out, condArg)
		}
		return append(out, extra...)
	}

	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM emails e WHERE `+cond, args()...).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(`
SELECT e.id, e.from_addr, e.to_addrs, e.subject, e.received_at,
       EXISTS (SELECT 1 FROM mail_reads m WHERE m.account_id = ? AND m.email_id = e.id),
       EXISTS (SELECT 1 FROM attachments t WHERE t.email_id = e.id)
FROM emails e WHERE `+cond+`
ORDER BY e.received_at DESC, e.id DESC LIMIT ? OFFSET ?`,
		append([]any{readAccountID}, args(limit, offset)...)...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	mails := []MailSummary{}
	for rows.Next() {
		var m MailSummary
		var to string
		if err := rows.Scan(&m.ID, &m.From, &to, &m.Subject, &m.ReceivedAt, &m.IsRead, &m.HasAttachments); err != nil {
			return nil, 0, err
		}
		_ = json.Unmarshal([]byte(to), &m.To)
		m.To = nonNil(m.To)
		mails = append(mails, m)
	}
	return mails, total, rows.Err()
}

func (s *Store) getMail(cond string, condArg, readAccountID int64, id string) (Mail, error) {
	args := []any{readAccountID, id}
	if strings.Contains(cond, "?") {
		args = append(args, condArg)
	}
	var m Mail
	var to, cc string
	err := s.db.QueryRow(`
SELECT e.id, e.from_addr, e.to_addrs, e.cc_addrs, e.subject, e.html, e.text, e.received_at,
       EXISTS (SELECT 1 FROM mail_reads m WHERE m.account_id = ? AND m.email_id = e.id)
FROM emails e WHERE e.id = ? AND `+cond, args...,
	).Scan(&m.ID, &m.From, &to, &cc, &m.Subject, &m.HTML, &m.Text, &m.ReceivedAt, &m.IsRead)
	if errors.Is(err, sql.ErrNoRows) {
		return Mail{}, ErrNotFound
	}
	if err != nil {
		return Mail{}, err
	}
	_ = json.Unmarshal([]byte(to), &m.To)
	_ = json.Unmarshal([]byte(cc), &m.Cc)
	m.To, m.Cc = nonNil(m.To), nonNil(m.Cc)

	rows, err := s.db.Query(`SELECT id, filename, content_type FROM attachments WHERE email_id = ? ORDER BY rowid`, id)
	if err != nil {
		return Mail{}, err
	}
	defer rows.Close()
	m.Attachments = []Attachment{}
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.ID, &a.Filename, &a.ContentType); err != nil {
			return Mail{}, err
		}
		m.Attachments = append(m.Attachments, a)
	}
	m.HasAttachments = len(m.Attachments) > 0
	return m, rows.Err()
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
