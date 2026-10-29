// Package middleware 提供会话鉴权、panic 恢复与写请求的 Content-Type 校验。
package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"mime"
	"net/http"
	"slices"

	"xemail/internal/service"
)

const CookieName = "xemail_session"

type identityKey struct{}

// IdentityFrom 取出 RequireRole 注入的调用者身份。
func IdentityFrom(r *http.Request) (service.Identity, bool) {
	id, ok := r.Context().Value(identityKey{}).(service.Identity)
	return id, ok
}

// RequireRole 校验会话并限定角色；roles 为空表示任意已登录角色。
// 未登录返回 401，角色不符返回 403。
func RequireRole(svc *service.Service, roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(CookieName)
			if err != nil {
				writeError(w, http.StatusUnauthorized, service.ErrUnauthorized.Error())
				return
			}
			id, err := svc.Authenticate(cookie.Value)
			if err != nil {
				if errors.Is(err, service.ErrUnauthorized) {
					writeError(w, http.StatusUnauthorized, err.Error())
					return
				}
				log.Printf("authenticate: %v", err)
				writeError(w, http.StatusInternalServerError, "服务器内部错误")
				return
			}
			if len(roles) > 0 && !slices.Contains(roles, id.Role) {
				writeError(w, http.StatusForbidden, "权限不足")
				return
			}
			ctx := context.WithValue(r.Context(), identityKey{}, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireJSON 要求写请求（POST/PUT/PATCH/DELETE）声明 application/json，
// 与 SameSite=Lax 配合拦截跨站表单提交。
func RequireJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || mt != "application/json" {
				writeError(w, http.StatusUnsupportedMediaType, "Content-Type 必须为 application/json")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				log.Printf("panic: %v", v)
				writeError(w, http.StatusInternalServerError, "服务器内部错误")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
