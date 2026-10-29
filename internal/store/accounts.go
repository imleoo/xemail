package store

import (
	"database/sql"
	"errors"
)

var (
	ErrNotFound  = errors.New("记录不存在")
	ErrDuplicate = errors.New("记录已存在")
	ErrStale     = errors.New("账号状态已变更")
)

type Account struct {
	ID                int64  `json:"id"`
	Address           string `json:"address"`
	PasswordHash      string `json:"-"`
	Enabled           bool   `json:"enabled"`
	Note              string `json:"note"`
	CreatedAt         int64  `json:"created_at"`
	PasswordUpdatedAt int64  `json:"password_updated_at"`
	LastLoginAt       *int64 `json:"last_login_at"`
}

const accountColumns = `id, address, password_hash, enabled, note, created_at, password_updated_at, last_login_at`

func scanAccount(row interface{ Scan(...any) error }) (Account, error) {
	var a Account
	var enabled int
	err := row.Scan(&a.ID, &a.Address, &a.PasswordHash, &enabled, &a.Note, &a.CreatedAt, &a.PasswordUpdatedAt, &a.LastLoginAt)
	a.Enabled = enabled == 1
	return a, err
}

func (s *Store) CreateAccount(address, passwordHash, note string, now int64) (Account, error) {
	res, err := s.db.Exec(
		`INSERT INTO accounts (address, password_hash, note, created_at, password_updated_at) VALUES (?, ?, ?, ?, ?)`,
		address, passwordHash, note, now, now,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Account{}, ErrDuplicate
		}
		return Account{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Account{}, err
	}
	return s.GetAccountByID(id)
}

func (s *Store) GetAccountByID(id int64) (Account, error) {
	a, err := scanAccount(s.db.QueryRow(`SELECT `+accountColumns+` FROM accounts WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	return a, err
}

func (s *Store) GetAccountByAddress(address string) (Account, error) {
	a, err := scanAccount(s.db.QueryRow(`SELECT `+accountColumns+` FROM accounts WHERE address = ?`, address))
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	return a, err
}

func (s *Store) ListAccounts() ([]Account, error) {
	rows, err := s.db.Query(`SELECT ` + accountColumns + ` FROM accounts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	accounts := []Account{}
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

// SetPassword 更新密码哈希，并在同一事务内删除该账号全部会话。
func (s *Store) SetPassword(id int64, passwordHash string, now int64) error {
	return s.inTx(func(tx *sql.Tx) error {
		res, err := tx.Exec(`UPDATE accounts SET password_hash = ?, password_updated_at = ? WHERE id = ?`, passwordHash, now, id)
		if err != nil {
			return err
		}
		if err := requireAffected(res); err != nil {
			return err
		}
		_, err = tx.Exec(`DELETE FROM sessions WHERE account_id = ?`, id)
		return err
	})
}

// SetEnabled 更新启用状态；禁用时在同一事务内删除该账号全部会话。
func (s *Store) SetEnabled(id int64, enabled bool) error {
	return s.inTx(func(tx *sql.Tx) error {
		flag := 0
		if enabled {
			flag = 1
		}
		res, err := tx.Exec(`UPDATE accounts SET enabled = ? WHERE id = ?`, flag, id)
		if err != nil {
			return err
		}
		if err := requireAffected(res); err != nil {
			return err
		}
		if !enabled {
			_, err = tx.Exec(`DELETE FROM sessions WHERE account_id = ?`, id)
		}
		return err
	})
}

func (s *Store) SetNote(id int64, note string) error {
	res, err := s.db.Exec(`UPDATE accounts SET note = ? WHERE id = ?`, note, id)
	if err != nil {
		return err
	}
	return requireAffected(res)
}

// DeleteAccount 删除账号，会话通过外键级联删除。
func (s *Store) DeleteAccount(id int64) error {
	res, err := s.db.Exec(`DELETE FROM accounts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return requireAffected(res)
}

func (s *Store) TouchLogin(id, now int64) error {
	_, err := s.db.Exec(`UPDATE accounts SET last_login_at = ? WHERE id = ?`, now, id)
	return err
}

func requireAffected(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// inTx 在事务内执行 fn。单连接模式下 fn 内只能使用 tx，不能再用 s.db，否则死锁。
func (s *Store) inTx(fn func(tx *sql.Tx) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
