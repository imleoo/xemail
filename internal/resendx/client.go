// Package resendx 封装 Resend SDK，业务层只依赖这里，便于替换与 mock。
package resendx

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/resend/resend-go/v4"
)

const maxRateLimitRetries = 3

type Client struct {
	rc *resend.Client
}

func New(apiKey string) *Client {
	return &Client{rc: resend.NewClient(apiKey)}
}

type Attachment struct {
	ID          string
	Filename    string
	ContentType string
}

type ReceivedPage struct {
	IDs     []string
	HasMore bool
}

type Received struct {
	ID          string
	From        string
	To          []string
	Cc          []string
	Bcc         []string
	ReceivedFor []string
	Subject     string
	HTML        string
	Text        string
	MessageID   string
	ReceivedAt  time.Time
	Attachments []Attachment
}

// ListReceived 按时间倒序列出已收邮件的 id；after 为上一页最后一个 id。
func (c *Client) ListReceived(ctx context.Context, after string, limit int) (ReceivedPage, error) {
	opts := &resend.ListOptions{Limit: &limit}
	if after != "" {
		opts.After = &after
	}
	resp, err := withRetry(ctx, func() (resend.ListReceivedEmailsResponse, error) {
		return c.rc.Emails.Receiving.ListWithOptions(ctx, opts)
	})
	if err != nil {
		return ReceivedPage{}, err
	}
	page := ReceivedPage{HasMore: resp.HasMore, IDs: make([]string, 0, len(resp.Data))}
	for _, e := range resp.Data {
		page.IDs = append(page.IDs, e.Id)
	}
	return page, nil
}

func (c *Client) GetReceived(ctx context.Context, id string) (Received, error) {
	e, err := withRetry(ctx, func() (*resend.ReceivedEmail, error) {
		return c.rc.Emails.Receiving.GetWithContext(ctx, id)
	})
	if err != nil {
		return Received{}, err
	}
	out := Received{
		ID: e.Id, From: e.From, To: e.To, Cc: e.Cc, Bcc: e.Bcc, ReceivedFor: e.ReceivedFor,
		Subject: e.Subject, HTML: e.Html, Text: e.Text, MessageID: e.MessageId,
		ReceivedAt: parseTime(e.CreatedAt),
	}
	for _, a := range e.Attachments {
		out.Attachments = append(out.Attachments, Attachment{ID: a.Id, Filename: a.Filename, ContentType: a.ContentType})
	}
	return out, nil
}

// AttachmentURL 返回附件的临时下载地址。
func (c *Client) AttachmentURL(ctx context.Context, emailID, attachmentID string) (string, error) {
	a, err := withRetry(ctx, func() (*resend.EmailAttachment, error) {
		return c.rc.Emails.Receiving.GetAttachmentWithContext(ctx, emailID, attachmentID)
	})
	if err != nil {
		return "", err
	}
	return a.DownloadUrl, nil
}

func parseTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07", "2006-01-02 15:04:05-07"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Now()
}

// withRetry 遇到 429 时按 Retry-After 等待后重试，最多 maxRateLimitRetries 次。
func withRetry[T any](ctx context.Context, call func() (T, error)) (T, error) {
	for attempt := 0; ; attempt++ {
		v, err := call()
		var rl *resend.RateLimitError
		if err == nil || !errors.As(err, &rl) || attempt >= maxRateLimitRetries {
			return v, err
		}
		wait := time.Second
		if secs, convErr := strconv.Atoi(rl.RetryAfter); convErr == nil && secs > 0 {
			wait = time.Duration(secs) * time.Second
		}
		select {
		case <-ctx.Done():
			return v, ctx.Err()
		case <-time.After(wait):
		}
	}
}

// VerifyWebhook 校验 Resend（Svix）Webhook 签名与 5 分钟时间窗；payload 必须是未经解析的原始请求体。
func VerifyWebhook(secret, id, timestamp, signature string, payload []byte) error {
	return resend.NewClient("").Webhooks.Verify(&resend.VerifyWebhookOptions{
		Payload:       string(payload),
		Headers:       resend.WebhookHeaders{Id: id, Timestamp: timestamp, Signature: signature},
		WebhookSecret: secret,
	})
}
