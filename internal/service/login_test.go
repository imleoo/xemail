package service

import (
	"errors"
	"testing"
)

func TestFifthAttemptCanSucceedAndClearsCounter(t *testing.T) {
	svc, _ := newTestService(t, "")
	_, pw, _ := svc.CreateAccount("a@example.com", "")

	for i := 0; i < 4; i++ {
		svc.LoginUser("1.1.1.1", "a@example.com", "wrong")
	}
	if _, _, err := svc.LoginUser("1.1.1.1", "a@example.com", pw); err != nil {
		t.Fatalf("第 5 次用正确密码应成功: %v", err)
	}

	// 成功后计数清零：再失败 4 次不应锁定
	for i := 0; i < 4; i++ {
		if _, _, err := svc.LoginUser("1.1.1.1", "a@example.com", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("清零后第 %d 次失败 err = %v", i+1, err)
		}
	}
	if _, _, err := svc.LoginUser("1.1.1.1", "a@example.com", pw); err != nil {
		t.Fatalf("清零后仍应可登录: %v", err)
	}
}

// 大小写和首尾空白不同的同一地址必须共用一个限流 key。
func TestAddressVariantsShareLimiter(t *testing.T) {
	svc, _ := newTestService(t, "")
	_, pw, _ := svc.CreateAccount("a@example.com", "")

	for _, v := range []string{"A@Example.com", " a@example.com ", "a@EXAMPLE.COM", "a@example.com", "A@example.com"} {
		svc.LoginUser("1.1.1.1", v, "wrong")
	}
	if _, _, err := svc.LoginUser("1.1.1.1", "a@example.com", pw); !errors.Is(err, ErrLocked) {
		t.Fatalf("err = %v, 期望 ErrLocked", err)
	}
}

func TestAllFailureModesReturnSameError(t *testing.T) {
	svc, _ := newTestService(t, "")
	_, pw, err := svc.CreateAccount("a@example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	disabled, dpw, err := svc.CreateAccount("off@example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	off := false
	if _, err := svc.UpdateAccount(disabled.ID, &off, nil); err != nil {
		t.Fatal(err)
	}

	cases := map[string][2]string{
		"账号不存在": {"nobody@example.com", "x"},
		"密码错误":  {"a@example.com", "x"},
		"账号禁用":  {"off@example.com", dpw},
	}
	for name, c := range cases {
		_, _, err := svc.LoginUser("1.1.1.1", c[0], c[1])
		if err != ErrInvalidCredentials {
			t.Errorf("%s: err = %v, 期望恰好为 ErrInvalidCredentials", name, err)
		}
	}
	if _, _, err := svc.LoginUser("2.2.2.2", "a@example.com", pw); err != nil {
		t.Fatalf("正确密码应可登录: %v", err)
	}
}

func TestDeleteAccountKillsSessions(t *testing.T) {
	svc, _ := newTestService(t, "")
	acct, pw, _ := svc.CreateAccount("a@example.com", "")
	token, _, _ := svc.LoginUser("1.1.1.1", "a@example.com", pw)

	if err := svc.DeleteAccount(acct.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("删除账号后会话应失效")
	}
	if err := svc.DeleteAccount(acct.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("重复删除 err = %v, 期望 ErrNotFound", err)
	}
}
