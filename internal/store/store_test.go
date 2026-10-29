package store

import (
	"errors"
	"path/filepath"
	"testing"

	"xemail/migrations"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Migrate(migrations.FS); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestMigrateIdempotent(t *testing.T) {
	st := newTestStore(t)
	if err := st.Migrate(migrations.FS); err != nil {
		t.Fatalf("第二次迁移失败: %v", err)
	}
}

func TestAccountAddressUnique(t *testing.T) {
	st := newTestStore(t)
	if _, err := st.CreateAccount("a@example.com", "h", "", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAccount("a@example.com", "h", "", 1); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("err = %v, 期望 ErrDuplicate", err)
	}
}

func TestDeleteAccountCascadesSessions(t *testing.T) {
	st := newTestStore(t)
	acct, _ := st.CreateAccount("a@example.com", "h", "", 1)
	if err := st.CreateSession(Session{TokenHash: "t1", Role: "user", AccountID: &acct.ID, ExpiresAt: 100}); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteAccount(acct.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetSession("t1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("会话应被级联删除, err = %v", err)
	}
}

func TestSetPasswordAndDisableDeleteSessions(t *testing.T) {
	st := newTestStore(t)
	acct, _ := st.CreateAccount("a@example.com", "h", "", 1)
	mk := func(h string) {
		if err := st.CreateSession(Session{TokenHash: h, Role: "user", AccountID: &acct.ID, ExpiresAt: 100}); err != nil {
			t.Fatal(err)
		}
	}

	mk("t1")
	if err := st.SetPassword(acct.ID, "h2", 2); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetSession("t1"); !errors.Is(err, ErrNotFound) {
		t.Fatal("重置密码后会话应失效")
	}

	mk("t2")
	if err := st.SetEnabled(acct.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetSession("t2"); !errors.Is(err, ErrNotFound) {
		t.Fatal("禁用后会话应失效")
	}

	if err := st.SetPassword(999, "h", 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("不存在的账号 err = %v, 期望 ErrNotFound", err)
	}
}

func TestReserveAttempt(t *testing.T) {
	st := newTestStore(t)
	const window, lock = 900, 900
	now := int64(1000)

	for i := 1; i <= 5; i++ {
		ok, err := st.ReserveAttempt("k", now, window, 5, lock)
		if err != nil || !ok {
			t.Fatalf("第 %d 次应允许, ok=%v err=%v", i, ok, err)
		}
	}
	if ok, _ := st.ReserveAttempt("k", now+1, window, 5, lock); ok {
		t.Fatal("第 6 次应被锁定")
	}
	if ok, _ := st.ReserveAttempt("other", now+1, window, 5, lock); !ok {
		t.Fatal("其他 key 不应受影响")
	}
	if ok, _ := st.ReserveAttempt("k", now+lock+1, window, 5, lock); !ok {
		t.Fatal("锁定过期后应重新允许")
	}

	if err := st.ClearAttempts("k"); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 5; i++ {
		if ok, _ := st.ReserveAttempt("k", now+5000, window, 5, lock); !ok {
			t.Fatalf("清零后第 %d 次应允许", i)
		}
	}
}

func TestReserveAttemptWindowResets(t *testing.T) {
	st := newTestStore(t)
	for i := 0; i < 4; i++ {
		st.ReserveAttempt("k", 1000, 900, 5, 900)
	}
	// 窗口外的新尝试从 1 重新计数，不会和之前 4 次叠加触发锁定
	for i := 0; i < 4; i++ {
		if ok, _ := st.ReserveAttempt("k", 5000, 900, 5, 900); !ok {
			t.Fatalf("窗口重置后第 %d 次应允许", i+1)
		}
	}
}
