package handler

import (
	"io/fs"
	"net/http"
	"strings"

	"xemail/internal/middleware"
	"xemail/internal/service"
)

type Deps struct {
	Service *service.Service
	Inbox   *service.Inbox
	Static  fs.FS
	Webhook http.Handler // 为 nil 时不注册 Webhook 路由（未配置签名密钥）
}

// NewRouter 组装 /api 路由，其余请求交给前端 SPA 静态资源。
func NewRouter(d Deps) http.Handler {
	authH := &authHandler{svc: d.Service}
	accountsH := &accountsHandler{svc: d.Service}
	domainsH := &domainsHandler{svc: d.Service}
	mailsH := &mailsHandler{inbox: d.Inbox}
	adminMailsH := &adminMailsHandler{inbox: d.Inbox}

	anySession := middleware.RequireRole(d.Service)
	adminOnly := middleware.RequireRole(d.Service, service.RoleAdmin)
	userOnly := middleware.RequireRole(d.Service, service.RoleUser)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/healthz", healthz)

	mux.HandleFunc("POST /api/auth/login", authH.login)
	mux.HandleFunc("POST /api/auth/admin-login", authH.adminLogin)
	mux.HandleFunc("POST /api/auth/logout", authH.logout)
	mux.Handle("GET /api/me", anySession(http.HandlerFunc(authH.me)))

	mux.Handle("GET /api/admin/accounts", adminOnly(http.HandlerFunc(accountsH.list)))
	mux.Handle("POST /api/admin/accounts", adminOnly(http.HandlerFunc(accountsH.create)))
	mux.Handle("POST /api/admin/accounts/{id}/reset-password", adminOnly(http.HandlerFunc(accountsH.resetPassword)))
	mux.Handle("PATCH /api/admin/accounts/{id}", adminOnly(http.HandlerFunc(accountsH.update)))
	mux.Handle("DELETE /api/admin/accounts/{id}", adminOnly(http.HandlerFunc(accountsH.remove)))

	mux.Handle("GET /api/admin/domains", adminOnly(http.HandlerFunc(domainsH.list)))
	mux.Handle("POST /api/admin/domains", adminOnly(http.HandlerFunc(domainsH.create)))
	mux.Handle("DELETE /api/admin/domains/{name}", adminOnly(http.HandlerFunc(domainsH.remove)))

	mux.Handle("GET /api/mails", userOnly(http.HandlerFunc(mailsH.list)))
	mux.Handle("GET /api/mails/{id}", userOnly(http.HandlerFunc(mailsH.get)))
	mux.Handle("GET /api/mails/{id}/attachments/{aid}", userOnly(http.HandlerFunc(mailsH.attachment)))

	mux.Handle("GET /api/admin/mails/unrouted", adminOnly(http.HandlerFunc(adminMailsH.listUnrouted)))
	mux.Handle("GET /api/admin/mails/unrouted/{id}", adminOnly(http.HandlerFunc(adminMailsH.getUnrouted)))
	mux.Handle("GET /api/admin/mails/unrouted/{id}/attachments/{aid}", adminOnly(http.HandlerFunc(adminMailsH.attachment)))
	mux.Handle("POST /api/admin/sync", adminOnly(http.HandlerFunc(adminMailsH.startSync)))
	mux.Handle("GET /api/admin/sync", adminOnly(http.HandlerFunc(adminMailsH.syncStatus)))

	if d.Webhook != nil {
		mux.Handle("POST /api/webhooks/resend", d.Webhook)
	}

	mux.Handle("/", spa(d.Static))

	return middleware.Recover(middleware.RequireJSON(mux))
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// spa 优先返回真实文件，找不到时回退 index.html 以支持前端路由。
func spa(static fs.FS) http.Handler {
	files := http.FileServerFS(static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			files.ServeHTTP(w, r)
			return
		}
		if _, err := fs.Stat(static, path); err != nil {
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
}
