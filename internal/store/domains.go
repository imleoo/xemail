package store

import (
	"database/sql"
	"errors"
)

var ErrDomainInUse = errors.New("域名下仍有账号")

type Domain struct {
	Name      string `json:"name"`
	CreatedAt int64  `json:"created_at"`
	Accounts  int    `json:"accounts"`
}

// accountDomain 取账号地址 @ 之后的域名部分；地址在创建时已校验只含一个 @。
const accountDomain = `substr(address, instr(address, '@') + 1)`

func (s *Store) ListDomains() ([]Domain, error) {
	rows, err := s.db.Query(`SELECT d.name, d.created_at,
		(SELECT COUNT(*) FROM accounts WHERE ` + accountDomain + ` = d.name)
		FROM domains d ORDER BY d.created_at, d.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	domains := []Domain{}
	for rows.Next() {
		var d Domain
		if err := rows.Scan(&d.Name, &d.CreatedAt, &d.Accounts); err != nil {
			return nil, err
		}
		domains = append(domains, d)
	}
	return domains, rows.Err()
}

func (s *Store) AddDomain(name string, now int64) error {
	_, err := s.db.Exec(`INSERT INTO domains (name, created_at) VALUES (?, ?)`, name, now)
	if isUniqueViolation(err) {
		return ErrDuplicate
	}
	return err
}

// SeedDomain 仅在域名表为空时写入，用于把旧的 XEMAIL_DOMAIN 配置带入数据库。
func (s *Store) SeedDomain(name string, now int64) error {
	_, err := s.db.Exec(`INSERT INTO domains (name, created_at) SELECT ?, ? WHERE NOT EXISTS (SELECT 1 FROM domains)`, name, now)
	return err
}

// DeleteDomain 删除域名；仍有账号使用该域名时返回 ErrDomainInUse。
func (s *Store) DeleteDomain(name string) error {
	return s.inTx(func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM accounts WHERE `+accountDomain+` = ?`, name).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrDomainInUse
		}
		res, err := tx.Exec(`DELETE FROM domains WHERE name = ?`, name)
		if err != nil {
			return err
		}
		return requireAffected(res)
	})
}
