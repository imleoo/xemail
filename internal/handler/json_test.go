package handler

import (
	"net/http/httptest"
	"testing"
)

func TestClientIPTrustsRealIPOnlyFromLoopback(t *testing.T) {
	cases := []struct {
		name       string
		remoteAddr string
		realIP     string
		want       string
	}{
		{"回环来源采信 X-Real-IP", "127.0.0.1:5000", "203.0.113.7", "203.0.113.7"},
		{"IPv6 回环来源采信 X-Real-IP", "[::1]:5000", "203.0.113.7", "203.0.113.7"},
		{"回环来源但头不合法则回退", "127.0.0.1:5000", "not-an-ip", "127.0.0.1"},
		{"回环来源无该头", "127.0.0.1:5000", "", "127.0.0.1"},
		{"非回环来源伪造该头无效", "198.51.100.9:5000", "203.0.113.7", "198.51.100.9"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.remoteAddr
		if c.realIP != "" {
			r.Header.Set("X-Real-IP", c.realIP)
		}
		if got := clientIP(r); got != c.want {
			t.Errorf("%s: clientIP = %q, 期望 %q", c.name, got, c.want)
		}
	}
}
