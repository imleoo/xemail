package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"sync"
	"time"

	"xemail/internal/resendx"
	"xemail/internal/store"
)

const (
	listPageSize   = 100
	maxListPages   = 20 // 单次同步最多列出 2000 封，防止异常数据拖垮同步
	defaultPace    = 600 * time.Millisecond
	defaultPageLen = 50
	maxPageLen     = 100
)

var (
	ErrSyncRunning  = errors.New("同步正在进行中")
	ErrMailNotFound = errors.New("邮件不存在")
)

// Source 是收信数据来源，生产环境由 resendx.Client 实现，测试用假实现替换。
type Source interface {
	ListReceived(ctx context.Context, after string, limit int) (resendx.ReceivedPage, error)
	GetReceived(ctx context.Context, id string) (resendx.Received, error)
	AttachmentURL(ctx context.Context, emailID, attachmentID string) (string, error)
}

type SyncStatus struct {
	Running        bool   `json:"running"`
	LastStartedAt  int64  `json:"last_started_at"`
	LastFinishedAt int64  `json:"last_finished_at"`
	LastError      string `json:"last_error"`
	Listed         int    `json:"listed"`
	Fetched        int    `json:"fetched"`
	Failed         int    `json:"failed"`
}

type Inbox struct {
	ctx   context.Context
	store *store.Store
	src   Source
	pace  time.Duration // 相邻两次 Resend 请求的间隔，避免触发限流
	now   func() time.Time

	mu     sync.Mutex
	status SyncStatus
	rerun  bool // 同步进行中又收到触发，本轮结束后需要再同步一次
}

// NewInbox 的 ctx 用于控制后台同步的生命周期，应为应用级 context。
func NewInbox(ctx context.Context, st *store.Store, src Source) *Inbox {
	return &Inbox{ctx: ctx, store: st, src: src, pace: defaultPace, now: time.Now}
}

func (in *Inbox) Status() SyncStatus {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.status
}

// StartSync 在后台启动一次同步；已有同步在进行时返回 ErrSyncRunning。
func (in *Inbox) StartSync() error {
	if !in.begin() {
		return ErrSyncRunning
	}
	go in.run()
	return nil
}

// Trigger 请求尽快同步一次（Webhook 到达时调用），不阻塞。
// 已有同步在进行时它可能已经列完邮件、漏掉新邮件，所以记下标记，本轮结束后立刻再同步一次。
func (in *Inbox) Trigger() {
	in.mu.Lock()
	if in.status.Running {
		in.rerun = true
		in.mu.Unlock()
		return
	}
	in.beginLocked()
	in.mu.Unlock()
	go in.run()
}

// Sync 同步执行一次，返回本次错误；已有同步在进行时返回 ErrSyncRunning。
func (in *Inbox) Sync() error {
	if !in.begin() {
		return ErrSyncRunning
	}
	return in.run()
}

// RunLoop 立即同步一次，之后每隔 interval 同步一次，直到 ctx 结束。
func (in *Inbox) RunLoop(interval time.Duration) {
	_ = in.StartSync()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-in.ctx.Done():
			return
		case <-t.C:
			_ = in.StartSync()
		}
	}
}

func (in *Inbox) begin() bool {
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.status.Running {
		return false
	}
	in.beginLocked()
	return true
}

func (in *Inbox) beginLocked() {
	in.status.Running = true
	in.status.LastStartedAt = in.now().Unix()
}

func (in *Inbox) run() error {
	listed, fetched, failed, err := in.work()

	in.mu.Lock()
	in.status.Running = false
	in.status.LastFinishedAt = in.now().Unix()
	in.status.Listed, in.status.Fetched, in.status.Failed = listed, fetched, failed
	in.status.LastError = ""
	if err != nil {
		in.status.LastError = err.Error()
	}
	again := in.rerun
	in.rerun = false
	in.mu.Unlock()

	if again && in.ctx.Err() == nil {
		_ = in.StartSync()
	}
	return err
}

func (in *Inbox) work() (listed, fetched, failed int, err error) {
	ids, err := in.listAll()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("列出邮件失败: %w", err)
	}
	listed = len(ids)
	existing, err := in.store.ExistingEmailIDs(ids)
	if err != nil {
		return listed, 0, 0, err
	}

	var firstErr error
	for _, id := range ids { // ids 为新到旧，新邮件优先入库
		if existing[id] {
			continue
		}
		if err := in.wait(); err != nil {
			return listed, fetched, failed, err
		}
		e, err := in.src.GetReceived(in.ctx, id)
		if err == nil {
			err = in.store.InsertEmail(toNewMail(e))
		}
		if err != nil {
			failed++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		fetched++
	}
	if failed > 0 {
		return listed, fetched, failed, fmt.Errorf("%d 封邮件拉取失败，首个错误: %w", failed, firstErr)
	}
	return listed, fetched, failed, nil
}

func (in *Inbox) listAll() ([]string, error) {
	var ids []string
	after := ""
	for page := 0; page < maxListPages; page++ {
		if page > 0 {
			if err := in.wait(); err != nil {
				return nil, err
			}
		}
		p, err := in.src.ListReceived(in.ctx, after, listPageSize)
		if err != nil {
			return nil, err
		}
		ids = append(ids, p.IDs...)
		if !p.HasMore || len(p.IDs) == 0 {
			break
		}
		after = p.IDs[len(p.IDs)-1]
	}
	return ids, nil
}

func (in *Inbox) wait() error {
	if in.pace <= 0 {
		return in.ctx.Err()
	}
	select {
	case <-in.ctx.Done():
		return in.ctx.Err()
	case <-time.After(in.pace):
		return nil
	}
}

func toNewMail(e resendx.Received) store.NewMail {
	m := store.NewMail{
		ID: e.ID, From: e.From, To: e.To, Cc: e.Cc,
		Subject: e.Subject, HTML: e.HTML, Text: e.Text, MessageID: e.MessageID,
		ReceivedAt: e.ReceivedAt.Unix(),
		// received_for 记录实际投递到的地址（含 Bcc），再并上邮件头里的 to/cc/bcc。
		Recipients: normalizeRecipients(e.ReceivedFor, e.To, e.Cc, e.Bcc),
	}
	for _, a := range e.Attachments {
		m.Attachments = append(m.Attachments, store.Attachment{ID: a.ID, Filename: a.Filename, ContentType: a.ContentType})
	}
	return m
}

// normalizeRecipients 去掉显示名（"Name <a@b.com>"）、统一小写并去重。
func normalizeRecipients(lists ...[]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, list := range lists {
		for _, raw := range list {
			addr := strings.ToLower(strings.TrimSpace(raw))
			if parsed, err := mail.ParseAddress(raw); err == nil {
				addr = strings.ToLower(parsed.Address)
			}
			if !strings.Contains(addr, "@") || seen[addr] {
				continue
			}
			seen[addr] = true
			out = append(out, addr)
		}
	}
	return out
}

func clampPage(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = defaultPageLen
	}
	return min(limit, maxPageLen), max(offset, 0)
}

func (in *Inbox) ListMails(accountID int64, limit, offset int) ([]store.MailSummary, int, error) {
	limit, offset = clampPage(limit, offset)
	return in.store.ListAccountMails(accountID, limit, offset)
}

// OpenMail 返回账号自己的邮件详情并标记已读；邮件不属于该账号时返回 ErrMailNotFound。
func (in *Inbox) OpenMail(accountID int64, id string) (store.Mail, error) {
	m, err := in.store.GetAccountMail(accountID, id)
	if err != nil {
		return store.Mail{}, mapMailNotFound(err)
	}
	if err := in.store.MarkRead(accountID, id); err != nil {
		return store.Mail{}, err
	}
	m.IsRead = true
	return m, nil
}

func (in *Inbox) ListUnrouted(limit, offset int) ([]store.MailSummary, int, error) {
	limit, offset = clampPage(limit, offset)
	return in.store.ListUnroutedMails(limit, offset)
}

func (in *Inbox) GetUnrouted(id string) (store.Mail, error) {
	m, err := in.store.GetUnroutedMail(id)
	return m, mapMailNotFound(err)
}

// UserAttachmentURL 校验邮件归属后返回附件的临时下载地址。
func (in *Inbox) UserAttachmentURL(accountID int64, mailID, attachmentID string) (string, error) {
	if _, err := in.store.GetAccountMail(accountID, mailID); err != nil {
		return "", mapMailNotFound(err)
	}
	return in.attachmentURL(mailID, attachmentID)
}

func (in *Inbox) AdminAttachmentURL(mailID, attachmentID string) (string, error) {
	if _, err := in.store.GetUnroutedMail(mailID); err != nil {
		return "", mapMailNotFound(err)
	}
	return in.attachmentURL(mailID, attachmentID)
}

func (in *Inbox) attachmentURL(mailID, attachmentID string) (string, error) {
	if _, err := in.store.GetAttachment(mailID, attachmentID); err != nil {
		return "", mapMailNotFound(err)
	}
	url, err := in.src.AttachmentURL(in.ctx, mailID, attachmentID)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(url, "https://") {
		return "", fmt.Errorf("附件下载地址不是 https: %q", url)
	}
	return url, nil
}

func mapMailNotFound(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return ErrMailNotFound
	}
	return err
}
