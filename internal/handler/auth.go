package handler

import (
	"net/http"
	"time"

	"xemail/internal/middleware"
	"xemail/internal/service"
)

type authHandler struct {
	svc *service.Service
}

func (h *authHandler) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Address  string `json:"address"`
		Password string `json:"password"`
	}
	if !decode(w, r, &req) {
		return
	}
	token, acct, err := h.svc.LoginUser(clientIP(r), req.Address, req.Password)
	if err != nil {
		fail(w, err)
		return
	}
	setSessionCookie(w, r, token, service.UserSessionTTL)
	writeJSON(w, http.StatusOK, map[string]string{"role": service.RoleUser, "address": acct.Address})
}

func (h *authHandler) adminLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &req) {
		return
	}
	token, err := h.svc.LoginAdmin(clientIP(r), req.Token)
	if err != nil {
		fail(w, err)
		return
	}
	setSessionCookie(w, r, token, service.AdminSessionTTL)
	writeJSON(w, http.StatusOK, map[string]string{"role": service.RoleAdmin})
}

// logout 幂等：没有会话或会话已失效时同样清除 Cookie 并返回 204。
func (h *authHandler) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(middleware.CookieName); err == nil {
		if err := h.svc.Logout(cookie.Value); err != nil {
			fail(w, err)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name: middleware.CookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: isSecure(r),
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *authHandler) me(w http.ResponseWriter, r *http.Request) {
	id, _ := middleware.IdentityFrom(r)
	resp := map[string]string{"role": id.Role}
	if id.Account != nil {
		resp["address"] = id.Account.Address
	}
	writeJSON(w, http.StatusOK, resp)
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name: middleware.CookieName, Value: token, Path: "/", MaxAge: int(ttl.Seconds()),
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: isSecure(r),
	})
}

func isSecure(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}
