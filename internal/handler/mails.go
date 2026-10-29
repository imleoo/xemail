package handler

import (
	"net/http"
	"strconv"

	"xemail/internal/middleware"
	"xemail/internal/service"
)

// mailsHandler 服务邮箱账号自己的收件箱。
type mailsHandler struct {
	inbox *service.Inbox
}

func (h *mailsHandler) list(w http.ResponseWriter, r *http.Request) {
	id, _ := middleware.IdentityFrom(r)
	limit, offset := pageParams(r)
	mails, total, err := h.inbox.ListMails(id.Account.ID, limit, offset)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mails": mails, "total": total})
}

func (h *mailsHandler) get(w http.ResponseWriter, r *http.Request) {
	id, _ := middleware.IdentityFrom(r)
	mail, err := h.inbox.OpenMail(id.Account.ID, r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mail)
}

func (h *mailsHandler) attachment(w http.ResponseWriter, r *http.Request) {
	id, _ := middleware.IdentityFrom(r)
	url, err := h.inbox.UserAttachmentURL(id.Account.ID, r.PathValue("id"), r.PathValue("aid"))
	redirectOrFail(w, r, url, err)
}

// adminMailsHandler 让管理员处理未匹配任何账号的邮件，并触发同步。
type adminMailsHandler struct {
	inbox *service.Inbox
}

func (h *adminMailsHandler) listUnrouted(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	mails, total, err := h.inbox.ListUnrouted(limit, offset)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mails": mails, "total": total})
}

func (h *adminMailsHandler) getUnrouted(w http.ResponseWriter, r *http.Request) {
	mail, err := h.inbox.GetUnrouted(r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mail)
}

func (h *adminMailsHandler) attachment(w http.ResponseWriter, r *http.Request) {
	url, err := h.inbox.AdminAttachmentURL(r.PathValue("id"), r.PathValue("aid"))
	redirectOrFail(w, r, url, err)
}

func (h *adminMailsHandler) startSync(w http.ResponseWriter, _ *http.Request) {
	if err := h.inbox.StartSync(); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, h.inbox.Status())
}

func (h *adminMailsHandler) syncStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.inbox.Status())
}

// redirectOrFail 跳转到附件的临时下载地址；该地址带签名，不允许被缓存或经 Referer 外泄。
func redirectOrFail(w http.ResponseWriter, r *http.Request, url string, err error) {
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, url, http.StatusFound)
}

func pageParams(r *http.Request) (limit, offset int) {
	limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ = strconv.Atoi(r.URL.Query().Get("offset"))
	return limit, offset
}
