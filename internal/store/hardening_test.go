package store

import (
	"errors"
	"sync"
	"testing"
)

func TestCreateUserSessionRejectsStaleAccount(t *testing.T) {
	st := newTestStore(t)
	acct, _ := st.CreateAccount("a@example.com", "h1", "", 1)
	sess := func(h string) Session {
		return Session{TokenHash: h, Role: "user", AccountID: &acct.ID, ExpiresAt: 100}
	}

	// 登录读取到 h1 后，管理员重置密码为 h2：用旧哈希建会话必须失败
	if err := st.SetPassword(acct.ID, "h2", 2); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateUserSession(acct.ID, "h1", sess("old")); !errors.Is(err, ErrStale) {
		t.Fatalf("旧哈希 err = %v, 期望 ErrStale", err)
	}
	if _, err := st.GetSession("old"); !errors.Is(err, ErrNotFound) {
		t.Fatal("过期凭证不应留下会话")
	}

	if err := st.CreateUserSession(acct.ID, "h2", sess("ok")); err != nil {
		t.Fatalf("当前哈希应成功: %v", err)
	}

	// 禁用后同样拒绝
	if err := st.SetEnabled(acct.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateUserSession(acct.ID, "h2", sess("disabled")); !errors.Is(err, ErrStale) {
		t.Fatalf("禁用账号 err = %v, 期望 ErrStale", err)
	}

	if err := st.CreateUserSession(9999, "h", sess("x")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("不存在账号 err = %v, 期望 ErrNotFound", err)
	}
}

func TestRevokeRemovesAllSessionsOfAccountOnly(t *testing.T) {
	st := newTestStore(t)
	a, _ := st.CreateAccount("a@example.com", "h", "", 1)
	b, _ := st.CreateAccount("b@example.com", "h", "", 1)
	for _, s := range []Session{
		{TokenHash: "a1", Role: "user", AccountID: &a.ID, ExpiresAt: 100},
		{TokenHash: "a2", Role: "user", AccountID: &a.ID, ExpiresAt: 100},
		{TokenHash: "b1", Role: "user", AccountID: &b.ID, ExpiresAt: 100},
	} {
		if err := st.CreateSession(s); err != nil {
			t.Fatal(err)
		}
	}

	if err := st.SetPassword(a.ID, "h2", 2); err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"a1", "a2"} {
		if _, err := st.GetSession(h); !errors.Is(err, ErrNotFound) {
			t.Errorf("会话 %s 应被删除", h)
		}
	}
	if _, err := st.GetSession("b1"); err != nil {
		t.Errorf("其他账号的会话不应受影响: %v", err)
	}
}

// 并发请求必须只有 max 个能拿到名额，否则限流可被并行请求绕过。
func TestReserveAttemptConcurrent(t *testing.T) {
	st := newTestStore(t)
	const goroutines, max = 40, 5

	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := st.ReserveAttempt("k", 1000, 900, max, 900)
			if err != nil {
				t.Error(err)
				return
			}
			if ok {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if allowed != max {
		t.Fatalf("并发下允许 %d 次, 期望 %d", allowed, max)
	}
}

func TestReserveAttemptUnlockBoundary(t *testing.T) {
	st := newTestStore(t)
	for i := 0; i < 5; i++ {
		st.ReserveAttempt("k", 1000, 900, 5, 900)
	} // 第 5 次后锁到 1900

	if ok, _ := st.ReserveAttempt("k", 1899, 900, 5, 900); ok {
		t.Fatal("锁定到期前一秒应被拒绝")
	}
	if ok, _ := st.ReserveAttempt("k", 1900, 900, 5, 900); !ok {
		t.Fatal("锁定到期瞬间应允许")
	}
}

// 固定当前语义：间隔不超过窗口的零散尝试会持续累计，只有连续 window 秒无尝试才清零。
func TestReserveAttemptSparseAttemptsAccumulate(t *testing.T) {
	st := newTestStore(t)
	for i, at := range []int64{0, 600, 1200, 1800, 2400} {
		if ok, _ := st.ReserveAttempt("k", at, 900, 5, 900); !ok {
			t.Fatalf("第 %d 次应允许", i+1)
		}
	}
	if ok, _ := st.ReserveAttempt("k", 2401, 900, 5, 900); ok {
		t.Fatal("第 6 次应被锁定")
	}
}

func TestPurgeAttemptsKeepsLockedRecords(t *testing.T) {
	st := newTestStore(t)
	for i := 0; i < 5; i++ {
		st.ReserveAttempt("locked", 5000, 900, 5, 900)
	}

	if err := st.PurgeAttempts(5100, 900); err != nil {
		t.Fatal(err)
	}
	if ok, _ := st.ReserveAttempt("locked", 5100, 900, 5, 900); ok {
		t.Error("锁定中的记录不应被清除")
	}
}
