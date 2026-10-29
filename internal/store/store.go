// Package store 封装 SQLite 访问，只含 SQL 读写，不含业务规则。
package store

import (
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

// Open 打开 SQLite 并启用外键、WAL 与忙等待。
func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite 单写者，限制为一个连接可避免 SQLITE_BUSY，并保证 pragma 对所有语句生效。
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Migrate 按文件名顺序执行未应用的迁移，每个迁移一个事务，重复调用幂等。
func (s *Store) Migrate(fsys fs.FS) error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY)`); err != nil {
		return err
	}
	names, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)

	for _, name := range names {
		var applied int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, name).Scan(&applied); err != nil {
			return err
		}
		if applied > 0 {
			continue
		}
		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		if err := s.applyMigration(name, string(body)); err != nil {
			return fmt.Errorf("迁移 %s 失败: %w", filepath.Base(name), err)
		}
	}
	return nil
}

func (s *Store) applyMigration(name, body string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(body); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations (name) VALUES (?)`, name); err != nil {
		return err
	}
	return tx.Commit()
}

// isUniqueViolation 判断是否为唯一约束冲突。
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
