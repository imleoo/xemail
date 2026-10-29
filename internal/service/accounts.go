package service

import (
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"xemail/internal/auth"
	"xemail/internal/store"
)

const (
	maxAddressLen = 254
	maxLocalLen   = 64
	maxNoteRunes  = 200
)

var addressPattern = regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9\-]+(\.[a-z0-9\-]+)*\.[a-z]{2,}$`)

func normalizeAddress(address string) string {
	return strings.ToLower(strings.TrimSpace(address))
}

func (s *Service) validateAddress(address string) error {
	local, domain, _ := strings.Cut(address, "@")
	if len(address) > maxAddressLen || len(local) > maxLocalLen || !addressPattern.MatchString(address) {
		return ErrInvalidAddress
	}
	allowed, err := s.domainAllowed(domain)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrInvalidAddress
	}
	return nil
}

func validateNote(note string) error {
	if utf8.RuneCountInString(note) > maxNoteRunes {
		return ErrInvalidInput
	}
	return nil
}

// CreateAccount 创建账号并返回一次性明文密码；明文不落库、不写日志。
func (s *Service) CreateAccount(address, note string) (store.Account, string, error) {
	address = normalizeAddress(address)
	if err := s.validateAddress(address); err != nil {
		return store.Account{}, "", err
	}
	if err := validateNote(note); err != nil {
		return store.Account{}, "", err
	}
	password, hash, err := newPassword()
	if err != nil {
		return store.Account{}, "", err
	}
	acct, err := s.store.CreateAccount(address, hash, note, s.now().Unix())
	if errors.Is(err, store.ErrDuplicate) {
		return store.Account{}, "", ErrConflict
	}
	if err != nil {
		return store.Account{}, "", err
	}
	return acct, password, nil
}

// ResetPassword 生成新密码并使该账号全部会话失效，返回一次性明文密码。
func (s *Service) ResetPassword(id int64) (string, error) {
	password, hash, err := newPassword()
	if err != nil {
		return "", err
	}
	if err := s.store.SetPassword(id, hash, s.now().Unix()); err != nil {
		return "", mapNotFound(err)
	}
	return password, nil
}

// UpdateAccount 修改启用状态与备注，nil 表示不修改；禁用时该账号全部会话失效。
func (s *Service) UpdateAccount(id int64, enabled *bool, note *string) (store.Account, error) {
	if note != nil {
		if err := validateNote(*note); err != nil {
			return store.Account{}, err
		}
	}
	if _, err := s.store.GetAccountByID(id); err != nil {
		return store.Account{}, mapNotFound(err)
	}
	if enabled != nil {
		if err := s.store.SetEnabled(id, *enabled); err != nil {
			return store.Account{}, mapNotFound(err)
		}
	}
	if note != nil {
		if err := s.store.SetNote(id, *note); err != nil {
			return store.Account{}, mapNotFound(err)
		}
	}
	acct, err := s.store.GetAccountByID(id)
	return acct, mapNotFound(err)
}

func (s *Service) DeleteAccount(id int64) error {
	return mapNotFound(s.store.DeleteAccount(id))
}

func (s *Service) ListAccounts() ([]store.Account, error) {
	return s.store.ListAccounts()
}

func newPassword() (plain, hash string, err error) {
	plain, err = auth.Generate()
	if err != nil {
		return "", "", err
	}
	hash, err = auth.Hash(plain)
	return plain, hash, err
}

func mapNotFound(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
