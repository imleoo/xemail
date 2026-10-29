package service

import (
	"errors"
	"regexp"
	"strings"

	"xemail/internal/store"
)

const maxDomainLen = 253

var domainPattern = regexp.MustCompile(`^([a-z0-9]([a-z0-9\-]*[a-z0-9])?\.)+[a-z]{2,}$`)

func normalizeDomain(name string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(name)), "@")
}

func validateDomain(name string) error {
	if len(name) > maxDomainLen || !domainPattern.MatchString(name) {
		return ErrInvalidDomain
	}
	return nil
}

func (s *Service) ListDomains() ([]store.Domain, error) {
	return s.store.ListDomains()
}

func (s *Service) AddDomain(name string) (store.Domain, error) {
	name = normalizeDomain(name)
	if err := validateDomain(name); err != nil {
		return store.Domain{}, err
	}
	now := s.now().Unix()
	if err := s.store.AddDomain(name, now); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			return store.Domain{}, ErrDomainExists
		}
		return store.Domain{}, err
	}
	return store.Domain{Name: name, CreatedAt: now}, nil
}

func (s *Service) DeleteDomain(name string) error {
	err := s.store.DeleteDomain(normalizeDomain(name))
	switch {
	case errors.Is(err, store.ErrNotFound):
		return ErrDomainNotFound
	case errors.Is(err, store.ErrDomainInUse):
		return ErrDomainInUse
	}
	return err
}

// SeedDomain 在域名表为空时写入 name（来自 XEMAIL_DOMAIN），name 为空则什么都不做。
func (s *Service) SeedDomain(name string) error {
	name = normalizeDomain(name)
	if name == "" {
		return nil
	}
	if err := validateDomain(name); err != nil {
		return err
	}
	return s.store.SeedDomain(name, s.now().Unix())
}

// domainAllowed 域名表为空时不限制；否则地址的域名必须在表中。
func (s *Service) domainAllowed(domain string) (bool, error) {
	domains, err := s.store.ListDomains()
	if err != nil {
		return false, err
	}
	if len(domains) == 0 {
		return true, nil
	}
	for _, d := range domains {
		if d.Name == domain {
			return true, nil
		}
	}
	return false, nil
}
