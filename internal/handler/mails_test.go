package handler

import (
	"context"
	"net/http"
	"testing"
	"time"

	"xemail/internal/resendx"
	"xemail/internal/store"
)

// fakeSource 不含任何邮件，附件固定返回一个 https 地址。
type fakeSource struct{}

func (fakeSource) ListReceived(context.Context, string, int) (resendx.ReceivedPage, error) {
	return resendx.ReceivedPage{}, nil
}

func (fakeSource) GetReceived(context.Context, string) (resendx.Received, error) {
	return resendx.Received{}, nil
}

func (fakeSource) AttachmentURL(context.Context, string, string) (string, error) {
	return "https://files.example/signed", nil
}

// userClient 创建账号并返回已登录的客户端。
func (e *env) userClient(t *testing.T, admin *http.Client, address string) *http.Client {
	t.Helper()
	_, created, _ := e.do(t, admin, "POST", "/api/admin/accounts", map[string]string{"address": address})
	c := e.client()
	if code, _, _ := e.do(t, c, "POST", "/api/auth/login", map[string]string{"address": address, "password": created["password"].(string)}); code != 200 {
		t.Fatalf("登录 %s = %d", address, code)
	}
	return c
}

func (e *env) addMail(t *testing.T, id string, withAttachment bool, recipients ...string) {
	t.Helper()
	m := store.NewMail{
		ID: id, From: "s@other.com", To: recipients, Subject: "subject " + id, Text: "body " + id,
		HTML: "<p>html " + id + "</p>", ReceivedAt: time.Now().Unix(), Recipients: recipients,
	}
	if withAttachment {
		m.Attachments = []store.Attachment{{ID: "att1", Filename: "f.pdf", ContentType: "application/pdf"}}
	}
	if err := e.st.InsertEmail(m); err != nil {
		t.Fatal(err)
	}
}

func mailIDs(body map[string]any) []string {
	var out []string
	for _, m := range body["mails"].([]any) {
		out = append(out, m.(map[string]any)["id"].(string))
	}
	return out
}

func TestUserSeesOnlyOwnMail(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient(t)
	alice := e.userClient(t, admin, "alice@example.com")
	bob := e.userClient(t, admin, "bob@example.com")
	e.addMail(t, "for-alice", false, "alice@example.com")
	e.addMail(t, "for-bob", false, "bob@example.com")

	_, body, _ := e.do(t, alice, "GET", "/api/mails", nil)
	if got := mailIDs(body); len(got) != 1 || got[0] != "for-alice" {
		t.Fatalf("alice 收件箱 = %v", got)
	}
	if code, _, _ := e.do(t, alice, "GET", "/api/mails/for-bob", nil); code != 404 {
		t.Errorf("读取他人邮件 = %d, 期望 404", code)
	}
	if code, _, _ := e.do(t, bob, "GET", "/api/mails/for-bob", nil); code != 200 {
		t.Errorf("读取自己的邮件 = %d, 期望 200", code)
	}
}

func TestMailEndpointsRequireUserRole(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient(t)
	e.addMail(t, "m1", false, "a@example.com")

	if code, _, _ := e.do(t, e.client(), "GET", "/api/mails", nil); code != 401 {
		t.Errorf("未登录 = %d, 期望 401", code)
	}
	if code, _, _ := e.do(t, admin, "GET", "/api/mails", nil); code != 403 {
		t.Errorf("管理员访问用户收件箱 = %d, 期望 403", code)
	}
	user := e.userClient(t, admin, "a@example.com")
	for _, p := range []string{"/api/admin/mails/unrouted", "/api/admin/sync"} {
		if code, _, _ := e.do(t, user, "GET", p, nil); code != 403 {
			t.Errorf("user 访问 %s = %d, 期望 403", p, code)
		}
	}
	if code, _, _ := e.do(t, user, "POST", "/api/admin/sync", nil); code != 403 {
		t.Errorf("user 触发同步 = %d, 期望 403", code)
	}
}

func TestOpeningMailMarksItRead(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient(t)
	alice := e.userClient(t, admin, "alice@example.com")
	e.addMail(t, "m1", false, "alice@example.com")

	isRead := func() bool {
		_, body, _ := e.do(t, alice, "GET", "/api/mails", nil)
		return body["mails"].([]any)[0].(map[string]any)["is_read"].(bool)
	}
	if isRead() {
		t.Fatal("新邮件应为未读")
	}
	_, detail, _ := e.do(t, alice, "GET", "/api/mails/m1", nil)
	if detail["text"] != "body m1" || detail["html"] != "<p>html m1</p>" {
		t.Fatalf("详情 = %v", detail)
	}
	if !isRead() {
		t.Fatal("打开后应为已读")
	}
}

func TestAttachmentRedirectIsPrivate(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient(t)
	alice := e.userClient(t, admin, "alice@example.com")
	bob := e.userClient(t, admin, "bob@example.com")
	e.addMail(t, "m1", true, "alice@example.com")

	resp, err := noFollow(alice).Get(e.srv.URL + "/api/mails/m1/attachments/att1")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "https://files.example/signed" {
		t.Fatalf("状态 %d, Location %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	if resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("跳转响应必须禁止缓存和 Referer: %v", resp.Header)
	}

	if code, _, _ := e.do(t, bob, "GET", "/api/mails/m1/attachments/att1", nil); code != 404 {
		t.Errorf("他人附件 = %d, 期望 404", code)
	}
	if code, _, _ := e.do(t, alice, "GET", "/api/mails/m1/attachments/nope", nil); code != 404 {
		t.Errorf("不存在的附件 = %d, 期望 404", code)
	}
}

func TestAdminSeesOnlyUnroutedMail(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient(t)
	e.userClient(t, admin, "alice@example.com")
	e.addMail(t, "routed", false, "alice@example.com")
	e.addMail(t, "orphan", true, "ghost@example.com")

	_, body, _ := e.do(t, admin, "GET", "/api/admin/mails/unrouted", nil)
	if got := mailIDs(body); len(got) != 1 || got[0] != "orphan" {
		t.Fatalf("未匹配 = %v", got)
	}
	if code, _, _ := e.do(t, admin, "GET", "/api/admin/mails/unrouted/orphan", nil); code != 200 {
		t.Errorf("读取未匹配邮件 = %d, 期望 200", code)
	}
	if code, _, _ := e.do(t, admin, "GET", "/api/admin/mails/unrouted/routed", nil); code != 404 {
		t.Errorf("管理员不应读到已分发邮件 = %d, 期望 404", code)
	}

	resp, err := noFollow(admin).Get(e.srv.URL + "/api/admin/mails/unrouted/orphan/attachments/att1")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "https://files.example/signed" {
		t.Errorf("管理员附件: 状态 %d, Location %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	resp, err = noFollow(admin).Get(e.srv.URL + "/api/admin/mails/unrouted/routed/attachments/att1")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("管理员不应下载已分发邮件的附件 = %d, 期望 404", resp.StatusCode)
	}
}

// noFollow 复制客户端的 Cookie 罐，但不跟随重定向，用于断言跳转本身。
func noFollow(c *http.Client) *http.Client {
	return &http.Client{Jar: c.Jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func TestAdminSyncStartsAndReportsStatus(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient(t)

	if code, _, _ := e.do(t, admin, "POST", "/api/admin/sync", nil); code != 202 {
		t.Fatalf("启动同步 = %d, 期望 202", code)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, status, _ := e.do(t, admin, "GET", "/api/admin/sync", nil)
		if running, _ := status["running"].(bool); !running && status["last_finished_at"].(float64) > 0 {
			if status["last_error"] != "" {
				t.Fatalf("同步出错: %v", status["last_error"])
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("同步未在预期时间内结束: %v", status)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
