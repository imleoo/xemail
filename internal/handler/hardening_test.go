package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func (e *env) raw(t *testing.T, method, path, body string, hdr map[string]string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func TestRejectsTrailingJSON(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient(t)

	for _, body := range []string{
		`{"token":"x"}garbage`,
		`{"token":"x"}{"token":"y"}`,
		`{"token":"x"} {`,
	} {
		if resp, _ := e.raw(t, "POST", "/api/auth/admin-login", body, nil); resp.StatusCode != 400 {
			t.Errorf("%q = %d, 期望 400", body, resp.StatusCode)
		}
	}

	// 带尾随内容的创建请求不能产生账号
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/admin/accounts", bytes.NewBufferString(`{"address":"x@example.com"}garbage`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := admin.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("创建 = %d, 期望 400", resp.StatusCode)
	}
	_, list, _ := e.do(t, admin, "GET", "/api/admin/accounts", nil)
	if n := len(list["accounts"].([]any)); n != 0 {
		t.Fatalf("畸形请求不应创建账号, 实际 %d 个", n)
	}
}

func TestRejectsUnknownFields(t *testing.T) {
	e := newEnv(t)
	if resp, _ := e.raw(t, "POST", "/api/auth/admin-login", `{"token":"x","extra":1}`, nil); resp.StatusCode != 400 {
		t.Errorf("未知字段 = %d, 期望 400", resp.StatusCode)
	}
}

// 账号不存在、密码错误、账号被禁用：状态码和响应体必须逐字节一致，客户端无法区分。
func TestLoginFailuresAreIndistinguishable(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient(t)
	e.do(t, admin, "POST", "/api/admin/accounts", map[string]string{"address": "a@example.com"})
	_, off, _ := e.do(t, admin, "POST", "/api/admin/accounts", map[string]string{"address": "off@example.com"})
	offPW := off["password"].(string)
	offID := int(off["account"].(map[string]any)["id"].(float64))
	e.do(t, admin, "PATCH", "/api/admin/accounts/"+itoa(offID), map[string]any{"enabled": false})

	type result struct {
		status int
		body   string
	}
	try := func(addr, pw string) result {
		b, _ := json.Marshal(map[string]string{"address": addr, "password": pw})
		resp, body := e.raw(t, "POST", "/api/auth/login", string(b), nil)
		return result{resp.StatusCode, body}
	}

	unknown := try("nobody@example.com", "x")
	wrong := try("a@example.com", "x")
	disabled := try("off@example.com", offPW)
	if unknown.status != 401 {
		t.Fatalf("状态码 = %d, 期望 401", unknown.status)
	}
	if unknown != wrong || unknown != disabled {
		t.Fatalf("三种失败响应不一致:\n%+v\n%+v\n%+v", unknown, wrong, disabled)
	}
}

func TestSessionCookieLifetimeAndSecure(t *testing.T) {
	e := newEnv(t)
	body := `{"token":"` + adminToken + `"}`

	resp, _ := e.raw(t, "POST", "/api/auth/admin-login", body, nil)
	c := resp.Cookies()
	if len(c) != 1 || c[0].MaxAge != 12*3600 || c[0].Secure {
		t.Fatalf("admin Cookie = %+v, 期望 MaxAge=43200 且非 Secure", c)
	}

	resp, _ = e.raw(t, "POST", "/api/auth/admin-login", body, map[string]string{"X-Forwarded-Proto": "https"})
	c = resp.Cookies()
	if len(c) != 1 || !c[0].Secure {
		t.Fatalf("HTTPS 下 Cookie 应带 Secure: %+v", c)
	}
}

func TestUserSessionCookieLifetime(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient(t)
	_, created, _ := e.do(t, admin, "POST", "/api/admin/accounts", map[string]string{"address": "a@example.com"})
	b, _ := json.Marshal(map[string]string{"address": "a@example.com", "password": created["password"].(string)})

	resp, _ := e.raw(t, "POST", "/api/auth/login", string(b), nil)
	c := resp.Cookies()
	if len(c) != 1 || c[0].MaxAge != 7*24*3600 {
		t.Fatalf("user Cookie = %+v, 期望 MaxAge=604800", c)
	}
}

func TestExpiredCookieValueIsRejected(t *testing.T) {
	e := newEnv(t)
	resp, _ := e.raw(t, "GET", "/api/me", "", map[string]string{"Cookie": "xemail_session=not-a-real-token"})
	if resp.StatusCode != 401 {
		t.Fatalf("伪造会话 = %d, 期望 401", resp.StatusCode)
	}
}
