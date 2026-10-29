package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"

	"xemail/internal/service"
)

const maxBodyBytes = 1 << 20

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeSecret 用于返回一次性明文密码，禁止任何缓存。
func writeSecret(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, v)
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	// 第二次 Decode 必须读到 EOF，拒绝 `{...}garbage` 或多个 JSON 值。
	if err := dec.Decode(v); err != nil || dec.Decode(&struct{}{}) != io.EOF {
		writeError(w, http.StatusBadRequest, "请求体不是合法的 JSON")
		return false
	}
	return true
}

// fail 把业务错误映射为 HTTP 状态码；未知错误只记日志，不向客户端暴露细节。
func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidCredentials), errors.Is(err, service.ErrUnauthorized):
		writeError(w, http.StatusUnauthorized, err.Error())
	case errors.Is(err, service.ErrLocked):
		writeError(w, http.StatusTooManyRequests, err.Error())
	case errors.Is(err, service.ErrInvalidAddress), errors.Is(err, service.ErrInvalidInput), errors.Is(err, service.ErrInvalidDomain):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrConflict), errors.Is(err, service.ErrDomainExists), errors.Is(err, service.ErrDomainInUse):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrNotFound), errors.Is(err, service.ErrMailNotFound), errors.Is(err, service.ErrDomainNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrSyncRunning):
		writeError(w, http.StatusConflict, err.Error())
	default:
		log.Printf("internal error: %v", err)
		writeError(w, http.StatusInternalServerError, "服务器内部错误")
	}
}

// clientIP 返回客户端 IP。只有请求来自本机反向代理（回环地址）时才采信 X-Real-IP，
// 否则任何人都能伪造该头来绕过登录限流。
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		if real := net.ParseIP(r.Header.Get("X-Real-IP")); real != nil {
			return real.String()
		}
	}
	return host
}
