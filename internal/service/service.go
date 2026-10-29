// Package service 承载账号管理与登录的业务规则。
package service

import (
	"crypto/sha256"
	"time"

	"xemail/internal/auth"
	"xemail/internal/store"
)

const (
	maxAttempts     = 5
	attemptWindow   = 15 * 60
	lockDuration    = 15 * 60
	UserSessionTTL  = 7 * 24 * time.Hour
	AdminSessionTTL = 12 * time.Hour
)

type Service struct {
	store          *store.Store
	adminTokenHash [sha256.Size]byte
	dummyHash      string
	now            func() time.Time
}

func New(st *store.Store, adminToken string) *Service {
	return &Service{
		store:          st,
		adminTokenHash: sha256.Sum256([]byte(adminToken)),
		dummyHash:      auth.DummyHash(),
		now:            time.Now,
	}
}

// Cleanup 清理过期会话与过期的登录尝试记录。
func (s *Service) Cleanup() error {
	now := s.now().Unix()
	if err := s.store.DeleteExpiredSessions(now); err != nil {
		return err
	}
	return s.store.PurgeAttempts(now, attemptWindow)
}
