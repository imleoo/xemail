package auth

import (
	"encoding/base64"
	"testing"
)

func TestHashTokenKnownSHA256Vector(t *testing.T) {
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" // sha256("abc")
	if got := HashToken("abc"); got != want {
		t.Fatalf("HashToken = %s, 期望 %s", got, want)
	}
}

func TestNewTokenIs32Bytes(t *testing.T) {
	tok, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil || len(raw) != 32 {
		t.Fatalf("解码后长度 = %d, err = %v, 期望 32 字节", len(raw), err)
	}
}
