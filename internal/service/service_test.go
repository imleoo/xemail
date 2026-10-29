package service

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"xemail/internal/store"
	"xemail/migrations"
)

const testAdminToken = "test-admin-token-0123456789"

func newTestService(t *testing.T, domain string) (*Service, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Migrate(migrations.FS); err != nil {
		t.Fatal(err)
	}
	svc := New(st, testAdminToken)
	if err := svc.SeedDomain(domain); err != nil {
		t.Fatal(err)
	}
	return svc, st
}

func TestDomainWhitelistManagedAtRuntime(t *testing.T) {
	svc, _ := newTestService(t, "example.com")

	if _, _, err := svc.CreateAccount("a@second.example.org", ""); !errors.Is(err, ErrInvalidAddress) {
		t.Fatalf("未添加的域名 err = %v, 期望 ErrInvalidAddress", err)
	}
	d, err := svc.AddDomain("  @Second.EXAMPLE.org ")
	if err != nil || d.Name != "second.example.org" {
		t.Fatalf("AddDomain = %+v, %v", d, err)
	}
	if _, _, err := svc.CreateAccount("a@second.example.org", ""); err != nil {
		t.Fatalf("添加后应可创建: %v", err)
	}
	if _, _, err := svc.CreateAccount("b@example.com", ""); err != nil {
		t.Fatalf("原有域名应仍可用: %v", err)
	}

	if _, err := svc.AddDomain("SECOND.example.org"); !errors.Is(err, ErrDomainExists) {
		t.Errorf("重复域名 err = %v, 期望 ErrDomainExists", err)
	}
	for _, bad := range []string{"", "nodot", "a b.com", "-a.com", "a..com", "a.c", "x@y.com"} {
		if _, err := svc.AddDomain(bad); !errors.Is(err, ErrInvalidDomain) {
			t.Errorf("%q: err = %v, 期望 ErrInvalidDomain", bad, err)
		}
	}

	if err := svc.DeleteDomain("second.example.org"); !errors.Is(err, ErrDomainInUse) {
		t.Fatalf("有账号的域名 err = %v, 期望 ErrDomainInUse", err)
	}
	domains, _ := svc.ListDomains()
	counts := map[string]int{}
	for _, d := range domains {
		counts[d.Name] = d.Accounts
	}
	if counts["second.example.org"] != 1 || counts["example.com"] != 1 {
		t.Errorf("账号计数 = %v", counts)
	}
	accts, _ := svc.ListAccounts()
	for _, a := range accts {
		if a.Address == "a@second.example.org" {
			if err := svc.DeleteAccount(a.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := svc.DeleteDomain("second.example.org"); err != nil {
		t.Fatalf("无账号后应可删除: %v", err)
	}
	if err := svc.DeleteDomain("second.example.org"); !errors.Is(err, ErrDomainNotFound) {
		t.Errorf("重复删除 err = %v, 期望 ErrDomainNotFound", err)
	}
}

func TestSeedDomainOnlyWhenEmpty(t *testing.T) {
	svc, _ := newTestService(t, "")
	if _, _, err := svc.CreateAccount("a@anything.org", ""); err != nil {
		t.Fatalf("域名表为空时不应限制: %v", err)
	}
	if err := svc.SeedDomain("example.com"); err != nil {
		t.Fatal(err)
	}
	if err := svc.SeedDomain("other.com"); err != nil {
		t.Fatal(err)
	}
	domains, _ := svc.ListDomains()
	if len(domains) != 1 || domains[0].Name != "example.com" {
		t.Fatalf("域名 = %+v, 期望仅 example.com", domains)
	}
	if err := svc.SeedDomain("bad domain"); !errors.Is(err, ErrInvalidDomain) {
		t.Errorf("非法初始域名 err = %v", err)
	}
}

func TestCreateAccountPasswordLogsInAndIsNotStoredPlain(t *testing.T) {
	svc, st := newTestService(t, "example.com")
	acct, password, err := svc.CreateAccount("  Alice@Example.com ", "测试")
	if err != nil {
		t.Fatal(err)
	}
	if acct.Address != "alice@example.com" {
		t.Fatalf("地址未规范化: %q", acct.Address)
	}
	if got, _ := st.GetAccountByID(acct.ID); got.PasswordHash == password || got.PasswordHash == "" {
		t.Fatal("密码哈希异常")
	}
	token, got, err := svc.LoginUser("1.1.1.1", "ALICE@example.com", password)
	if err != nil || token == "" || got.ID != acct.ID {
		t.Fatalf("登录失败: %v", err)
	}
	id, err := svc.Authenticate(token)
	if err != nil || id.Role != RoleUser || id.Account.ID != acct.ID {
		t.Fatalf("Authenticate = %+v, %v", id, err)
	}
}

func TestCreateAccountValidation(t *testing.T) {
	svc, _ := newTestService(t, "example.com")
	for _, addr := range []string{"", "no-at", "a@other.com", "a@example.com.evil.com", "@example.com", "a b@example.com"} {
		if _, _, err := svc.CreateAccount(addr, ""); !errors.Is(err, ErrInvalidAddress) {
			t.Errorf("%q: err = %v, 期望 ErrInvalidAddress", addr, err)
		}
	}
	if _, _, err := svc.CreateAccount("a@example.com", ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.CreateAccount("A@example.com", ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("重复地址 err = %v, 期望 ErrConflict", err)
	}
}

func TestResetPasswordInvalidatesOldPasswordAndSessions(t *testing.T) {
	svc, _ := newTestService(t, "")
	acct, oldPw, _ := svc.CreateAccount("a@example.com", "")
	token, _, err := svc.LoginUser("1.1.1.1", "a@example.com", oldPw)
	if err != nil {
		t.Fatal(err)
	}

	newPw, err := svc.ResetPassword(acct.ID)
	if err != nil || newPw == oldPw {
		t.Fatalf("重置失败: %v", err)
	}
	if _, err := svc.Authenticate(token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("重置后旧会话应失效")
	}
	if _, _, err := svc.LoginUser("2.2.2.2", "a@example.com", oldPw); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatal("旧密码应失效")
	}
	if _, _, err := svc.LoginUser("2.2.2.2", "a@example.com", newPw); err != nil {
		t.Fatalf("新密码登录失败: %v", err)
	}
	if _, err := svc.ResetPassword(9999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, 期望 ErrNotFound", err)
	}
}

func TestDisabledAccountCannotLoginAndSessionsDie(t *testing.T) {
	svc, _ := newTestService(t, "")
	acct, pw, _ := svc.CreateAccount("a@example.com", "")
	token, _, _ := svc.LoginUser("1.1.1.1", "a@example.com", pw)

	off := false
	if _, err := svc.UpdateAccount(acct.ID, &off, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("禁用后会话应失效")
	}
	if _, _, err := svc.LoginUser("1.1.1.1", "a@example.com", pw); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("禁用账号 err = %v, 期望 ErrInvalidCredentials", err)
	}
}

func TestLoginLockoutAfterFiveFailures(t *testing.T) {
	svc, _ := newTestService(t, "")
	_, pw, _ := svc.CreateAccount("a@example.com", "")

	for i := 1; i <= 5; i++ {
		if _, _, err := svc.LoginUser("1.1.1.1", "a@example.com", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("第 %d 次 err = %v", i, err)
		}
	}
	if _, _, err := svc.LoginUser("1.1.1.1", "a@example.com", pw); !errors.Is(err, ErrLocked) {
		t.Fatalf("锁定后正确密码 err = %v, 期望 ErrLocked", err)
	}
	if _, _, err := svc.LoginUser("9.9.9.9", "a@example.com", pw); err != nil {
		t.Fatalf("其他 IP 不应被锁: %v", err)
	}

	svc.now = func() time.Time { return time.Now().Add(16 * time.Minute) }
	if _, _, err := svc.LoginUser("1.1.1.1", "a@example.com", pw); err != nil {
		t.Fatalf("锁定过期后应可登录: %v", err)
	}
}

func TestUnknownAccountAndWrongPasswordSameError(t *testing.T) {
	svc, _ := newTestService(t, "")
	svc.CreateAccount("a@example.com", "")

	_, _, errUnknown := svc.LoginUser("1.1.1.1", "nobody@example.com", "x")
	_, _, errWrong := svc.LoginUser("1.1.1.1", "a@example.com", "x")
	if errUnknown != errWrong || !errors.Is(errUnknown, ErrInvalidCredentials) {
		t.Fatalf("两种失败应返回同一错误: %v vs %v", errUnknown, errWrong)
	}
}

func TestAdminLoginAndLockout(t *testing.T) {
	svc, _ := newTestService(t, "")
	if _, err := svc.LoginAdmin("1.1.1.1", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("err = %v", err)
	}
	token, err := svc.LoginAdmin("1.1.1.1", testAdminToken)
	if err != nil {
		t.Fatal(err)
	}
	id, err := svc.Authenticate(token)
	if err != nil || id.Role != RoleAdmin || id.Account != nil {
		t.Fatalf("Authenticate = %+v, %v", id, err)
	}

	for i := 0; i < 5; i++ {
		svc.LoginAdmin("3.3.3.3", "wrong")
	}
	if _, err := svc.LoginAdmin("3.3.3.3", testAdminToken); !errors.Is(err, ErrLocked) {
		t.Fatalf("管理员锁定 err = %v, 期望 ErrLocked", err)
	}
}

func TestSessionExpiry(t *testing.T) {
	svc, _ := newTestService(t, "")
	token, _ := svc.LoginAdmin("1.1.1.1", testAdminToken)
	svc.now = func() time.Time { return time.Now().Add(AdminSessionTTL + time.Minute) }
	if _, err := svc.Authenticate(token); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("过期会话 err = %v", err)
	}
}

func TestLogout(t *testing.T) {
	svc, _ := newTestService(t, "")
	token, _ := svc.LoginAdmin("1.1.1.1", testAdminToken)
	if err := svc.Logout(token); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("登出后会话应失效")
	}
}
