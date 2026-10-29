package auth

import (
	"strings"
	"testing"
)

func TestGenerateLengthAndCharset(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		p, err := Generate()
		if err != nil {
			t.Fatal(err)
		}
		if len(p) != PasswordLength {
			t.Fatalf("长度 = %d, 期望 %d", len(p), PasswordLength)
		}
		for _, c := range p {
			if !strings.ContainsRune(passwordAlphabet, c) {
				t.Fatalf("出现字符集之外的字符 %q", c)
			}
		}
		if seen[p] {
			t.Fatalf("重复密码 %q", p)
		}
		seen[p] = true
	}
}

func TestAlphabetExcludesAmbiguous(t *testing.T) {
	if strings.ContainsAny(passwordAlphabet, "IlOo01") {
		t.Fatal("字符集包含易混淆字符")
	}
}

func TestHashVerify(t *testing.T) {
	h, err := Hash("S3cretPassw0rd")
	if err != nil {
		t.Fatal(err)
	}
	if h == "S3cretPassw0rd" {
		t.Fatal("哈希等于明文")
	}
	if !Verify(h, "S3cretPassw0rd") {
		t.Fatal("正确密码校验失败")
	}
	if Verify(h, "wrong") {
		t.Fatal("错误密码校验通过")
	}
}

func TestTokenHash(t *testing.T) {
	a, _ := NewToken()
	b, _ := NewToken()
	if a == b {
		t.Fatal("令牌重复")
	}
	if HashToken(a) != HashToken(a) || HashToken(a) == HashToken(b) {
		t.Fatal("HashToken 不稳定或碰撞")
	}
	if HashToken(a) == a {
		t.Fatal("哈希等于明文令牌")
	}
}
