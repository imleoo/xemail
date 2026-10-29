package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"xemail/internal/resendx"
	"xemail/internal/store"
)

// fakeSource 按"新到旧"提供邮件，并记录 GetReceived 的调用次数。
type fakeSource struct {
	mu       sync.Mutex
	emails   []resendx.Received
	pageCap  int // 每页最多返回多少条，用于模拟多页
	failGet  map[string]bool
	getCalls map[string]int
	block    chan struct{} // 非 nil 时 GetReceived 会阻塞到它被关闭
	attURL   string
}

func (f *fakeSource) ListReceived(_ context.Context, after string, limit int) (resendx.ReceivedPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	start := 0
	if after != "" {
		for i, e := range f.emails {
			if e.ID == after {
				start = i + 1
			}
		}
	}
	n := limit
	if f.pageCap > 0 {
		n = min(n, f.pageCap)
	}
	end := min(start+n, len(f.emails))
	page := resendx.ReceivedPage{HasMore: end < len(f.emails)}
	for _, e := range f.emails[start:end] {
		page.IDs = append(page.IDs, e.ID)
	}
	return page, nil
}

func (f *fakeSource) GetReceived(_ context.Context, id string) (resendx.Received, error) {
	if f.block != nil {
		<-f.block
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getCalls == nil {
		f.getCalls = map[string]int{}
	}
	f.getCalls[id]++
	if f.failGet[id] {
		return resendx.Received{}, errors.New("boom")
	}
	for _, e := range f.emails {
		if e.ID == id {
			return e, nil
		}
	}
	return resendx.Received{}, errors.New("not found")
}

func (f *fakeSource) AttachmentURL(_ context.Context, _, _ string) (string, error) {
	return f.attURL, nil
}

func rcv(id string, to ...string) resendx.Received {
	return resendx.Received{
		ID: id, From: "sender@other.com", To: to, Subject: "subject " + id,
		Text: "body " + id, ReceivedAt: time.Unix(1_700_000_000, 0),
	}
}

type inboxEnv struct {
	svc   *Service
	inbox *Inbox
	src   *fakeSource
}

func newInboxEnv(t *testing.T) *inboxEnv {
	t.Helper()
	svc, st := newTestService(t, "")
	src := &fakeSource{attURL: "https://files.example/a"}
	inbox := NewInbox(context.Background(), st, src)
	inbox.pace = 0
	return &inboxEnv{svc: svc, inbox: inbox, src: src}
}

func (e *inboxEnv) account(t *testing.T, address string) int64 {
	t.Helper()
	a, _, err := e.svc.CreateAccount(address, "")
	if err != nil {
		t.Fatal(err)
	}
	return a.ID
}

func ids(mails []store.MailSummary) []string {
	out := make([]string, len(mails))
	for i, m := range mails {
		out[i] = m.ID
	}
	slices.Sort(out)
	return out
}

func TestSyncStoresMailAndIsIdempotent(t *testing.T) {
	e := newInboxEnv(t)
	e.src.emails = []resendx.Received{rcv("m3", "a@example.com"), rcv("m2", "a@example.com"), rcv("m1", "a@example.com")}
	a := e.account(t, "a@example.com")

	if err := e.inbox.Sync(); err != nil {
		t.Fatal(err)
	}
	if s := e.inbox.Status(); s.Listed != 3 || s.Fetched != 3 || s.Failed != 0 || s.Running {
		t.Fatalf("status = %+v", s)
	}
	mails, total, _ := e.inbox.ListMails(a, 0, 0)
	if total != 3 || len(mails) != 3 {
		t.Fatalf("total=%d len=%d", total, len(mails))
	}

	if err := e.inbox.Sync(); err != nil {
		t.Fatal(err)
	}
	if s := e.inbox.Status(); s.Listed != 3 || s.Fetched != 0 {
		t.Fatalf("第二次同步不应重复拉取: %+v", s)
	}
	for id, n := range e.src.getCalls {
		if n != 1 {
			t.Errorf("%s 被拉取 %d 次, 期望 1", id, n)
		}
	}
}

func TestSyncFollowsPagination(t *testing.T) {
	e := newInboxEnv(t)
	e.src.pageCap = 2
	for i := 5; i >= 1; i-- {
		e.src.emails = append(e.src.emails, rcv(fmt.Sprintf("m%d", i), "a@example.com"))
	}
	if err := e.inbox.Sync(); err != nil {
		t.Fatal(err)
	}
	if s := e.inbox.Status(); s.Listed != 5 || s.Fetched != 5 {
		t.Fatalf("status = %+v", s)
	}
}

func TestSyncContinuesAfterFailureAndBackfillsLater(t *testing.T) {
	e := newInboxEnv(t)
	a := e.account(t, "a@example.com")
	e.src.emails = []resendx.Received{rcv("m3", "a@example.com"), rcv("m2", "a@example.com"), rcv("m1", "a@example.com")}
	e.src.failGet = map[string]bool{"m2": true}

	if err := e.inbox.Sync(); err == nil {
		t.Fatal("有邮件拉取失败时 Sync 应返回错误")
	}
	if s := e.inbox.Status(); s.Fetched != 2 || s.Failed != 1 || s.LastError == "" {
		t.Fatalf("status = %+v", s)
	}
	if _, total, _ := e.inbox.ListMails(a, 0, 0); total != 2 {
		t.Fatalf("其余邮件应已入库, total = %d", total)
	}

	// 失败的邮件比已入库的更旧：下次同步必须能补回，而不是因为"见到已存在的就停"而漏掉
	e.src.failGet = nil
	if err := e.inbox.Sync(); err != nil {
		t.Fatal(err)
	}
	if _, total, _ := e.inbox.ListMails(a, 0, 0); total != 3 {
		t.Fatalf("补拉后 total = %d, 期望 3", total)
	}
}

func TestRoutingByReceivedForToCcAndDisplayName(t *testing.T) {
	e := newInboxEnv(t)
	a := e.account(t, "a@example.com")
	b := e.account(t, "b@example.com")
	c := e.account(t, "c@example.com")

	withDisplay := rcv("display", `Alice <A@Example.COM>`)
	bcc := rcv("bcc", "someone@else.com")
	bcc.ReceivedFor = []string{"b@example.com"}
	cc := rcv("cc", "x@else.com")
	cc.Cc = []string{"c@example.com"}
	both := rcv("both", "a@example.com", "b@example.com")
	nobody := rcv("nobody", "ghost@example.com")
	e.src.emails = []resendx.Received{withDisplay, bcc, cc, both, nobody}

	if err := e.inbox.Sync(); err != nil {
		t.Fatal(err)
	}

	check := func(name string, accountID int64, want ...string) {
		t.Helper()
		mails, _, err := e.inbox.ListMails(accountID, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if got := ids(mails); !slices.Equal(got, want) {
			t.Errorf("%s 收件箱 = %v, 期望 %v", name, got, want)
		}
	}
	check("a", a, "both", "display")
	check("b", b, "bcc", "both")
	check("c", c, "cc")

	unrouted, total, _ := e.inbox.ListUnrouted(0, 0)
	if got := ids(unrouted); total != 1 || !slices.Equal(got, []string{"nobody"}) {
		t.Fatalf("未匹配 = %v (total %d), 期望 [nobody]", got, total)
	}
}

func TestAccountCreatedLaterSeesHistoricalMail(t *testing.T) {
	e := newInboxEnv(t)
	e.src.emails = []resendx.Received{rcv("old", "late@example.com")}
	if err := e.inbox.Sync(); err != nil {
		t.Fatal(err)
	}
	if _, total, _ := e.inbox.ListUnrouted(0, 0); total != 1 {
		t.Fatal("账号创建前邮件应为未匹配")
	}

	late := e.account(t, "late@example.com")
	if mails, _, _ := e.inbox.ListMails(late, 0, 0); len(mails) != 1 {
		t.Fatal("账号创建后应能看到历史邮件")
	}
	if _, total, _ := e.inbox.ListUnrouted(0, 0); total != 0 {
		t.Fatal("已被账号匹配的邮件不应再算未匹配")
	}
}

func TestOpenMailMarksReadAndEnforcesOwnership(t *testing.T) {
	e := newInboxEnv(t)
	a := e.account(t, "a@example.com")
	b := e.account(t, "b@example.com")
	e.src.emails = []resendx.Received{rcv("m1", "a@example.com")}
	e.inbox.Sync()

	if mails, _, _ := e.inbox.ListMails(a, 0, 0); mails[0].IsRead {
		t.Fatal("新邮件应为未读")
	}
	m, err := e.inbox.OpenMail(a, "m1")
	if err != nil || m.Text != "body m1" || !m.IsRead {
		t.Fatalf("OpenMail = %+v, %v", m, err)
	}
	if mails, _, _ := e.inbox.ListMails(a, 0, 0); !mails[0].IsRead {
		t.Fatal("打开后应为已读")
	}

	if _, err := e.inbox.OpenMail(b, "m1"); !errors.Is(err, ErrMailNotFound) {
		t.Fatalf("他人邮件 err = %v, 期望 ErrMailNotFound", err)
	}
	if _, err := e.inbox.OpenMail(a, "nope"); !errors.Is(err, ErrMailNotFound) {
		t.Fatalf("不存在邮件 err = %v", err)
	}
	// 管理员的"未匹配"入口不能读到已分发给账号的邮件
	if _, err := e.inbox.GetUnrouted("m1"); !errors.Is(err, ErrMailNotFound) {
		t.Fatalf("GetUnrouted 读到了已分发邮件: %v", err)
	}
}

func TestAttachmentURL(t *testing.T) {
	e := newInboxEnv(t)
	a := e.account(t, "a@example.com")
	b := e.account(t, "b@example.com")
	m := rcv("m1", "a@example.com")
	m.Attachments = []resendx.Attachment{{ID: "att1", Filename: "f.pdf", ContentType: "application/pdf"}}
	e.src.emails = []resendx.Received{m}
	e.inbox.Sync()

	if url, err := e.inbox.UserAttachmentURL(a, "m1", "att1"); err != nil || url != "https://files.example/a" {
		t.Fatalf("url=%q err=%v", url, err)
	}
	if _, err := e.inbox.UserAttachmentURL(b, "m1", "att1"); !errors.Is(err, ErrMailNotFound) {
		t.Fatalf("他人附件 err = %v", err)
	}
	if _, err := e.inbox.UserAttachmentURL(a, "m1", "other"); !errors.Is(err, ErrMailNotFound) {
		t.Fatalf("不存在的附件 err = %v", err)
	}

	e.src.attURL = "http://insecure.example/a"
	if _, err := e.inbox.UserAttachmentURL(a, "m1", "att1"); err == nil {
		t.Fatal("非 https 的下载地址必须被拒绝")
	}
}

func TestConcurrentSyncRejected(t *testing.T) {
	e := newInboxEnv(t)
	e.src.emails = []resendx.Received{rcv("m1", "a@example.com")}
	e.src.block = make(chan struct{})

	if err := e.inbox.StartSync(); err != nil {
		t.Fatal(err)
	}
	if err := e.inbox.StartSync(); !errors.Is(err, ErrSyncRunning) {
		t.Fatalf("第二次启动 err = %v, 期望 ErrSyncRunning", err)
	}
	if !e.inbox.Status().Running {
		t.Fatal("状态应为 Running")
	}

	close(e.src.block)
	deadline := time.Now().Add(5 * time.Second)
	for e.inbox.Status().Running {
		if time.Now().After(deadline) {
			t.Fatal("同步未在预期时间内结束")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if s := e.inbox.Status(); s.Fetched != 1 || s.LastError != "" {
		t.Fatalf("status = %+v", s)
	}
}

func (e *inboxEnv) waitFetched(t *testing.T, id string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		e.src.mu.Lock()
		n := e.src.getCalls[id]
		e.src.mu.Unlock()
		if n > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("邮件 %s 未在预期时间内被拉取", id)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestTriggerStartsSyncWhenIdle(t *testing.T) {
	e := newInboxEnv(t)
	e.src.emails = []resendx.Received{rcv("m1", "a@example.com")}

	e.inbox.Trigger()
	e.waitFetched(t, "m1")
}

// 同步已列完邮件后才到达的新邮件，必须由 Trigger 记下的重跑补上，而不是等下一次轮询。
func TestTriggerDuringSyncReruns(t *testing.T) {
	e := newInboxEnv(t)
	e.src.emails = []resendx.Received{rcv("m1", "a@example.com")}
	e.src.block = make(chan struct{})

	if err := e.inbox.StartSync(); err != nil {
		t.Fatal(err)
	}
	// 第一轮已经列完 [m1] 并阻塞在拉取上；此时 m2 到达并收到 Webhook。
	e.src.mu.Lock()
	e.src.emails = append([]resendx.Received{rcv("m2", "a@example.com")}, e.src.emails...)
	e.src.mu.Unlock()
	e.inbox.Trigger()

	close(e.src.block)
	e.waitFetched(t, "m2")
}

func TestNormalizeRecipients(t *testing.T) {
	got := normalizeRecipients(
		[]string{"B@Example.com", "  a@example.com "},
		[]string{`"Bob" <b@example.com>`, "not-an-address", ""},
	)
	if want := []string{"b@example.com", "a@example.com"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestClampPage(t *testing.T) {
	for _, c := range []struct{ limit, offset, wantLimit, wantOffset int }{
		{0, 0, 50, 0}, {-5, -1, 50, 0}, {10, 3, 10, 3}, {5000, 0, 100, 0},
	} {
		if l, o := clampPage(c.limit, c.offset); l != c.wantLimit || o != c.wantOffset {
			t.Errorf("clampPage(%d,%d) = %d,%d", c.limit, c.offset, l, o)
		}
	}
}
