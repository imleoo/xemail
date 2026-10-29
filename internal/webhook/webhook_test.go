package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"xemail/internal/store"
	"xemail/migrations"
)

var secretKey = []byte("0123456789abcdef0123456789abcdef")

const (
	received = `{"type":"email.received","data":{"email_id":"e1"}}`
	other    = `{"type":"email.bounced","data":{}}`
)

func secret() string { return "whsec_" + base64.StdEncoding.EncodeToString(secretKey) }

func sign(id string, ts int64, body string) string {
	mac := hmac.New(sha256.New, secretKey)
	mac.Write([]byte(id + "." + strconv.FormatInt(ts, 10) + "." + body))
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

type env struct {
	h        http.Handler
	triggers int
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
	e := &env{}
	e.h = New(secret(), st, func() { e.triggers++ })
	return e
}

func (e *env) post(id string, ts int64, sig, body string) int {
	req := httptest.NewRequest("POST", "/api/webhooks/resend", strings.NewReader(body))
	req.Header.Set("svix-id", id)
	req.Header.Set("svix-timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("svix-signature", sig)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec.Code
}

func TestValidEventTriggersSyncOnce(t *testing.T) {
	e := newEnv(t)
	ts := time.Now().Unix()
	if code := e.post("msg_1", ts, sign("msg_1", ts, received), received); code != 200 {
		t.Fatalf("状态码 = %d", code)
	}
	if e.triggers != 1 {
		t.Fatalf("triggers = %d, 期望 1", e.triggers)
	}
	// 同一 svix-id 重放：确认成功但不再触发
	if code := e.post("msg_1", ts, sign("msg_1", ts, received), received); code != 200 {
		t.Fatalf("重放状态码 = %d", code)
	}
	if e.triggers != 1 {
		t.Fatalf("重放后 triggers = %d, 期望仍为 1", e.triggers)
	}
	// 新的 svix-id 正常触发
	if code := e.post("msg_2", ts, sign("msg_2", ts, received), received); code != 200 || e.triggers != 2 {
		t.Fatalf("msg_2: code=%d triggers=%d", code, e.triggers)
	}
}

func TestRejectsBadSignatureTamperedBodyAndStaleTimestamp(t *testing.T) {
	e := newEnv(t)
	ts := time.Now().Unix()
	cases := map[string]struct {
		ts   int64
		sig  string
		body string
	}{
		"伪造签名":  {ts, "v1," + base64.StdEncoding.EncodeToString([]byte("forged")), received},
		"缺少签名":  {ts, "", received},
		"篡改正文":  {ts, sign("msg_1", ts, received), received + " "},
		"时间戳过期": {ts - 3600, sign("msg_1", ts-3600, received), received},
	}
	for name, c := range cases {
		if code := e.post("msg_1", c.ts, c.sig, c.body); code != 400 {
			t.Errorf("%s: 状态码 = %d, 期望 400", name, code)
		}
	}
	if e.triggers != 0 {
		t.Fatalf("验签失败不应触发同步, triggers = %d", e.triggers)
	}
}

func TestOtherEventTypesAckedWithoutTrigger(t *testing.T) {
	e := newEnv(t)
	ts := time.Now().Unix()
	if code := e.post("msg_3", ts, sign("msg_3", ts, other), other); code != 200 {
		t.Fatalf("状态码 = %d", code)
	}
	if e.triggers != 0 {
		t.Fatalf("triggers = %d, 期望 0", e.triggers)
	}
}
