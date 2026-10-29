// Package auth 提供随机密码、密码哈希与会话令牌原语，不依赖存储层。
package auth

import (
	"crypto/rand"
	"fmt"
	"math/big"

	"golang.org/x/crypto/bcrypt"
)

const (
	// 去掉易混淆的 I l O o 0 1。
	passwordAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789"
	PasswordLength   = 16
	bcryptCost       = 12
)

// Generate 返回 PasswordLength 位的随机密码。big.Int 方式取均匀随机数，没有取模偏差。
func Generate() (string, error) {
	max := big.NewInt(int64(len(passwordAlphabet)))
	out := make([]byte, PasswordLength)
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("生成随机密码失败: %w", err)
		}
		out[i] = passwordAlphabet[n.Int64()]
	}
	return string(out), nil
}

func Hash(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

func Verify(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// DummyHash 用于账号不存在时做一次等价的 bcrypt 比对，避免响应时间暴露账号是否存在。
func DummyHash() string {
	h, err := Hash("dummy-password-for-timing")
	if err != nil {
		panic(err)
	}
	return h
}
