package service

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"time"

	"xemail/internal/auth"
	"xemail/internal/store"
)

const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// Identity 是通过会话认证后的调用者身份。Account 仅 user 角色有值。
type Identity struct {
	Role    string
	Account *store.Account
}

// LoginUser 校验邮箱账号与密码，成功返回会话令牌。
// 账号不存在、密码错误、账号被禁用返回同一个 ErrInvalidCredentials。
func (s *Service) LoginUser(ip, address, password string) (string, store.Account, error) {
	_ = s.Cleanup()

	address = normalizeAddress(address)
	key := attemptKey(ip, address)
	ok, err := s.store.ReserveAttempt(key, s.now().Unix(), attemptWindow, maxAttempts, lockDuration)
	if err != nil {
		return "", store.Account{}, err
	}
	if !ok {
		return "", store.Account{}, ErrLocked
	}

	acct, err := s.store.GetAccountByAddress(address)
	if errors.Is(err, store.ErrNotFound) {
		auth.Verify(s.dummyHash, password)
		return "", store.Account{}, ErrInvalidCredentials
	}
	if err != nil {
		return "", store.Account{}, err
	}
	if !auth.Verify(acct.PasswordHash, password) || !acct.Enabled {
		return "", store.Account{}, ErrInvalidCredentials
	}

	// bcrypt 校验期间账号可能已被重置或禁用；建会话时在事务内重新确认，避免旧凭证换出有效会话。
	token, sess, err := s.buildSession(RoleUser, &acct.ID, UserSessionTTL)
	if err != nil {
		return "", store.Account{}, err
	}
	if err := s.store.CreateUserSession(acct.ID, acct.PasswordHash, sess); err != nil {
		if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrStale) {
			return "", store.Account{}, ErrInvalidCredentials
		}
		return "", store.Account{}, err
	}
	if err := s.store.ClearAttempts(key); err != nil {
		return "", store.Account{}, err
	}
	if err := s.store.TouchLogin(acct.ID, s.now().Unix()); err != nil {
		return "", store.Account{}, err
	}
	return token, acct, nil
}

// LoginAdmin 校验管理员令牌，成功返回会话令牌。
func (s *Service) LoginAdmin(ip, token string) (string, error) {
	_ = s.Cleanup()

	key := attemptKey(ip, "admin")
	ok, err := s.store.ReserveAttempt(key, s.now().Unix(), attemptWindow, maxAttempts, lockDuration)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrLocked
	}

	sum := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(sum[:], s.adminTokenHash[:]) != 1 {
		return "", ErrInvalidCredentials
	}
	if err := s.store.ClearAttempts(key); err != nil {
		return "", err
	}
	return s.newSession(RoleAdmin, nil, AdminSessionTTL)
}

// Authenticate 用会话令牌换取身份；令牌无效、过期、对应账号被禁用或删除时返回 ErrUnauthorized。
func (s *Service) Authenticate(token string) (Identity, error) {
	if token == "" {
		return Identity{}, ErrUnauthorized
	}
	hash := auth.HashToken(token)
	sess, err := s.store.GetSession(hash)
	if errors.Is(err, store.ErrNotFound) {
		return Identity{}, ErrUnauthorized
	}
	if err != nil {
		return Identity{}, err
	}
	if sess.ExpiresAt <= s.now().Unix() {
		_ = s.store.DeleteSession(hash)
		return Identity{}, ErrUnauthorized
	}
	if sess.Role == RoleAdmin {
		return Identity{Role: RoleAdmin}, nil
	}

	if sess.AccountID == nil {
		return Identity{}, ErrUnauthorized
	}
	acct, err := s.store.GetAccountByID(*sess.AccountID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !acct.Enabled) {
		_ = s.store.DeleteSession(hash)
		return Identity{}, ErrUnauthorized
	}
	if err != nil {
		return Identity{}, err
	}
	return Identity{Role: RoleUser, Account: &acct}, nil
}

func (s *Service) Logout(token string) error {
	return s.store.DeleteSession(auth.HashToken(token))
}

// buildSession 生成令牌与对应的会话记录，但不落库。
func (s *Service) buildSession(role string, accountID *int64, ttl time.Duration) (string, store.Session, error) {
	token, err := auth.NewToken()
	if err != nil {
		return "", store.Session{}, err
	}
	return token, store.Session{
		TokenHash: auth.HashToken(token),
		Role:      role,
		AccountID: accountID,
		ExpiresAt: s.now().Add(ttl).Unix(),
	}, nil
}

func (s *Service) newSession(role string, accountID *int64, ttl time.Duration) (string, error) {
	token, sess, err := s.buildSession(role, accountID, ttl)
	if err != nil {
		return "", err
	}
	if err := s.store.CreateSession(sess); err != nil {
		return "", err
	}
	return token, nil
}

// attemptKey 限制每个 key 的长度，避免攻击者用超长地址撑大限流表。
func attemptKey(ip, name string) string {
	if len(name) > 254 {
		name = name[:254]
	}
	return ip + "|" + name
}
