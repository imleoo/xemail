package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"xemail/internal/service"
	"xemail/internal/store"
	"xemail/migrations"
)

const adminToken = "test-admin-token-0123456789"

type env struct {
	srv *httptest.Server
	st  *store.Store
}

func newEnv(t *testing.T) *env {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Migrate(migrations.FS); err != nil {
		t.Fatal(err)
	}
	svc := service.New(st, adminToken)
	if err := svc.SeedDomain("example.com"); err != nil {
		t.Fatal(err)
	}
	static := fstest.MapFS{"index.html": {Data: []byte("<title>xemail</title>")}}
	inbox := service.NewInbox(context.Background(), st, fakeSource{})
	srv := httptest.NewServer(NewRouter(Deps{Service: svc, Inbox: inbox, Static: static}))
	t.Cleanup(srv.Close)
	return &env{srv: srv, st: st}
}

// client 返回带独立 Cookie 罐的客户端，模拟一个浏览器。
func (e *env) client() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

func (e *env) do(t *testing.T, c *http.Client, method, path string, body any) (int, map[string]any, http.Header) {
	t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, e.srv.URL+path, rd)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out, resp.Header
}

func (e *env) adminClient(t *testing.T) *http.Client {
	t.Helper()
	c := e.client()
	if code, _, _ := e.do(t, c, "POST", "/api/auth/admin-login", map[string]string{"token": adminToken}); code != 200 {
		t.Fatalf("管理员登录 = %d", code)
	}
	return c
}

func TestUnauthenticatedIs401(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	for _, p := range []string{"/api/me", "/api/admin/accounts", "/api/admin/domains"} {
		if code, _, _ := e.do(t, c, "GET", p, nil); code != 401 {
			t.Errorf("%s = %d, 期望 401", p, code)
		}
	}
}

func TestWriteRequestRequiresJSONContentType(t *testing.T) {
	e := newEnv(t)
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/auth/admin-login", strings.NewReader("token="+adminToken))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("状态码 = %d, 期望 415", resp.StatusCode)
	}
}

func TestAdminLoginWrongToken(t *testing.T) {
	e := newEnv(t)
	if code, _, _ := e.do(t, e.client(), "POST", "/api/auth/admin-login", map[string]string{"token": "nope"}); code != 401 {
		t.Fatalf("状态码 = %d, 期望 401", code)
	}
}

func TestDomainAdminFlow(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient(t)

	if code, _, _ := e.do(t, admin, "POST", "/api/admin/accounts", map[string]string{"address": "a@second.example.org"}); code != 400 {
		t.Fatalf("添加前 = %d, 期望 400", code)
	}
	if code, body, _ := e.do(t, admin, "POST", "/api/admin/domains", map[string]string{"name": "Second.example.org"}); code != 201 {
		t.Fatalf("添加域名 = %d %v", code, body)
	}
	if code, _, _ := e.do(t, admin, "POST", "/api/admin/domains", map[string]string{"name": "second.example.org"}); code != 409 {
		t.Errorf("重复域名 = %d, 期望 409", code)
	}
	if code, _, _ := e.do(t, admin, "POST", "/api/admin/domains", map[string]string{"name": "bad domain"}); code != 400 {
		t.Errorf("非法域名 = %d, 期望 400", code)
	}
	_, created, _ := e.do(t, admin, "POST", "/api/admin/accounts", map[string]string{"address": "a@second.example.org"})
	if created["account"] == nil {
		t.Fatalf("添加域名后应能创建账号: %v", created)
	}

	_, list, _ := e.do(t, admin, "GET", "/api/admin/domains", nil)
	if domains, _ := list["domains"].([]any); len(domains) != 2 {
		t.Fatalf("域名列表 = %v, 期望 2 个", list)
	}

	if code, _, _ := e.do(t, admin, "DELETE", "/api/admin/domains/second.example.org", nil); code != 409 {
		t.Errorf("删除有账号的域名 = %d, 期望 409", code)
	}
	if code, _, _ := e.do(t, admin, "DELETE", "/api/admin/domains/nope.com", nil); code != 404 {
		t.Errorf("删除不存在的域名 = %d, 期望 404", code)
	}

	user := e.client()
	if code, _, _ := e.do(t, user, "GET", "/api/admin/domains", nil); code != 401 {
		t.Errorf("未登录访问域名接口 = %d, 期望 401", code)
	}
}

func TestCreateAccountFlow(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient(t)

	code, body, hdr := e.do(t, admin, "POST", "/api/admin/accounts", map[string]string{"address": "Bob@example.com", "note": "n"})
	if code != 201 {
		t.Fatalf("创建 = %d %v", code, body)
	}
	if hdr.Get("Cache-Control") != "no-store" {
		t.Error("返回明文密码的响应必须 no-store")
	}
	password, _ := body["password"].(string)
	if len(password) != 16 {
		t.Fatalf("密码长度 = %d", len(password))
	}

	if code, _, _ := e.do(t, admin, "POST", "/api/admin/accounts", map[string]string{"address": "bob@example.com"}); code != 409 {
		t.Errorf("重复地址 = %d, 期望 409", code)
	}
	if code, _, _ := e.do(t, admin, "POST", "/api/admin/accounts", map[string]string{"address": "bob@other.com"}); code != 400 {
		t.Errorf("域名不符 = %d, 期望 400", code)
	}

	// 列表中不应出现密码或哈希
	_, list, _ := e.do(t, admin, "GET", "/api/admin/accounts", nil)
	raw, _ := json.Marshal(list)
	if strings.Contains(string(raw), password) || strings.Contains(string(raw), "password_hash") {
		t.Error("列表泄露了密码或哈希")
	}

	user := e.client()
	if code, _, _ := e.do(t, user, "POST", "/api/auth/login", map[string]string{"address": "bob@example.com", "password": password}); code != 200 {
		t.Fatalf("账号登录 = %d", code)
	}
	if _, me, _ := e.do(t, user, "GET", "/api/me", nil); me["role"] != "user" || me["address"] != "bob@example.com" {
		t.Errorf("/api/me = %v", me)
	}
	if code, _, _ := e.do(t, user, "GET", "/api/admin/accounts", nil); code != 403 {
		t.Errorf("user 访问 admin 路由 = %d, 期望 403", code)
	}
}

func TestResetPasswordKillsUserSession(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient(t)
	_, created, _ := e.do(t, admin, "POST", "/api/admin/accounts", map[string]string{"address": "a@example.com"})
	pw := created["password"].(string)
	id := int(created["account"].(map[string]any)["id"].(float64))

	user := e.client()
	e.do(t, user, "POST", "/api/auth/login", map[string]string{"address": "a@example.com", "password": pw})
	if code, _, _ := e.do(t, user, "GET", "/api/me", nil); code != 200 {
		t.Fatal("登录后应能访问 /api/me")
	}

	code, reset, hdr := e.do(t, admin, "POST", "/api/admin/accounts/"+itoa(id)+"/reset-password", nil)
	if code != 200 || reset["password"] == pw || hdr.Get("Cache-Control") != "no-store" {
		t.Fatalf("重置 = %d %v", code, reset)
	}
	if code, _, _ := e.do(t, user, "GET", "/api/me", nil); code != 401 {
		t.Errorf("重置后旧会话 = %d, 期望 401", code)
	}
}

func TestDisableAndDelete(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient(t)
	_, created, _ := e.do(t, admin, "POST", "/api/admin/accounts", map[string]string{"address": "a@example.com"})
	pw := created["password"].(string)
	path := "/api/admin/accounts/" + itoa(int(created["account"].(map[string]any)["id"].(float64)))

	if code, _, _ := e.do(t, admin, "PATCH", path, map[string]any{"enabled": false}); code != 200 {
		t.Fatalf("禁用 = %d", code)
	}
	if code, _, _ := e.do(t, e.client(), "POST", "/api/auth/login", map[string]string{"address": "a@example.com", "password": pw}); code != 401 {
		t.Errorf("禁用账号登录 = %d, 期望 401", code)
	}
	if code, _, _ := e.do(t, admin, "DELETE", path, nil); code != 204 {
		t.Fatalf("删除 = %d", code)
	}
	if code, _, _ := e.do(t, admin, "DELETE", path, nil); code != 404 {
		t.Errorf("重复删除 = %d, 期望 404", code)
	}
	if code, _, _ := e.do(t, admin, "DELETE", "/api/admin/accounts/abc", nil); code != 400 {
		t.Errorf("非法 id = %d, 期望 400", code)
	}
}

func TestLoginLockout429(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient(t)
	_, created, _ := e.do(t, admin, "POST", "/api/admin/accounts", map[string]string{"address": "a@example.com"})
	pw := created["password"].(string)

	c := e.client()
	for i := 0; i < 5; i++ {
		if code, _, _ := e.do(t, c, "POST", "/api/auth/login", map[string]string{"address": "a@example.com", "password": "wrong"}); code != 401 {
			t.Fatalf("第 %d 次失败 = %d, 期望 401", i+1, code)
		}
	}
	if code, _, _ := e.do(t, c, "POST", "/api/auth/login", map[string]string{"address": "a@example.com", "password": pw}); code != 429 {
		t.Errorf("锁定后正确密码 = %d, 期望 429", code)
	}
}

func TestLogoutIsIdempotentAndKillsSession(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient(t)
	if code, _, _ := e.do(t, admin, "POST", "/api/auth/logout", nil); code != 204 {
		t.Fatalf("登出 = %d", code)
	}
	if code, _, _ := e.do(t, admin, "GET", "/api/me", nil); code != 401 {
		t.Errorf("登出后 /api/me = %d, 期望 401", code)
	}
	if code, _, _ := e.do(t, e.client(), "POST", "/api/auth/logout", nil); code != 204 {
		t.Errorf("无会话登出 = %d, 期望 204", code)
	}
}

func TestSessionCookieFlags(t *testing.T) {
	e := newEnv(t)
	b, _ := json.Marshal(map[string]string{"token": adminToken})
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/auth/admin-login", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	cookies := resp.Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].Path != "/" {
		t.Fatalf("Cookie 属性不符: %+v", cookies)
	}
}

func TestSPAFallback(t *testing.T) {
	e := newEnv(t)
	resp, err := http.Get(e.srv.URL + "/inbox")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(buf.String(), "xemail") {
		t.Errorf("SPA 回退失败: %d %q", resp.StatusCode, buf.String())
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
