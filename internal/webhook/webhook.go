// Package webhook 接收 Resend 的 email.received 推送：验签、按 svix-id 去重、触发一次同步。
package webhook

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"xemail/internal/resendx"
	"xemail/internal/store"
)

const (
	maxBodyBytes       = 1 << 20
	eventEmailReceived = "email.received"
)

type handler struct {
	secret  string
	store   *store.Store
	trigger func()
}

// New 返回 Webhook 处理器；trigger 在收到新的 email.received 事件时调用，不得阻塞。
func New(secret string, st *store.Store, trigger func()) http.Handler {
	return &handler{secret: secret, store: st, trigger: trigger}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 验签必须用原始 body，所以先读完再验签，验签通过后才反序列化。
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	id := r.Header.Get("svix-id")
	if err := resendx.VerifyWebhook(h.secret, id, r.Header.Get("svix-timestamp"), r.Header.Get("svix-signature"), body); err != nil {
		http.Error(w, "invalid signature", http.StatusBadRequest)
		return
	}

	var event struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	// 其他事件类型直接确认，避免 Resend 因非 2xx 反复重试。
	if event.Type != eventEmailReceived {
		w.WriteHeader(http.StatusOK)
		return
	}

	fresh, err := h.store.RecordWebhookEvent(id, event.Type, time.Now().Unix())
	if err != nil {
		log.Printf("webhook: record event: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if fresh {
		h.trigger()
	}
	w.WriteHeader(http.StatusOK)
}
