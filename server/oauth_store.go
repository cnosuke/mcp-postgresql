package server

import (
	"context"
	"sync"
	"time"
)

const (
	pendingAuthzTTL = 10 * time.Minute
	authCodeTTL     = 5 * time.Minute
)

type PendingAuthorization struct {
	ClientID            string
	ClientName          string
	RedirectURI         string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
	Scope               string
	Resource            string
	GoogleState         string
	CSRFToken           string
	CreatedAt           time.Time
}

type AuthorizationCode struct {
	Code                string
	ClientID            string
	RedirectURI         string
	CodeChallenge       string
	CodeChallengeMethod string
	Resource            string
	UserID              string
	Email               string
	CreatedAt           time.Time
}

type OAuthStore struct {
	mu          sync.Mutex
	pendingAuthz map[string]*PendingAuthorization
	authCodes    map[string]*AuthorizationCode
	csrfTokens   map[string]string
}

func NewOAuthStore() *OAuthStore {
	return &OAuthStore{
		pendingAuthz: make(map[string]*PendingAuthorization),
		authCodes:    make(map[string]*AuthorizationCode),
		csrfTokens:   make(map[string]string),
	}
}

func (s *OAuthStore) StorePendingAuthorization(pa *PendingAuthorization) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pendingAuthz[pa.GoogleState] = pa
	s.csrfTokens[pa.CSRFToken] = pa.GoogleState
}

func (s *OAuthStore) ConsumePendingByCSRF(csrfToken string) *PendingAuthorization {
	s.mu.Lock()
	defer s.mu.Unlock()

	googleState, ok := s.csrfTokens[csrfToken]
	if !ok {
		return nil
	}
	delete(s.csrfTokens, csrfToken)
	pa := s.pendingAuthz[googleState]
	if pa != nil && time.Since(pa.CreatedAt) > pendingAuthzTTL {
		delete(s.pendingAuthz, googleState)
		return nil
	}
	return pa
}

func (s *OAuthStore) ConsumePendingByGoogleState(googleState string) *PendingAuthorization {
	s.mu.Lock()
	defer s.mu.Unlock()

	pa, ok := s.pendingAuthz[googleState]
	if !ok {
		return nil
	}
	delete(s.pendingAuthz, googleState)
	delete(s.csrfTokens, pa.CSRFToken)
	if time.Since(pa.CreatedAt) > pendingAuthzTTL {
		return nil
	}
	return pa
}

func (s *OAuthStore) StoreAuthCode(ac *AuthorizationCode) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.authCodes[ac.Code] = ac
}

func (s *OAuthStore) GetAuthCode(code string) *AuthorizationCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	ac, ok := s.authCodes[code]
	if !ok {
		return nil
	}
	if time.Since(ac.CreatedAt) > authCodeTTL {
		delete(s.authCodes, code)
		return nil
	}
	return ac
}

func (s *OAuthStore) ConsumeAuthCode(code string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.authCodes, code)
}

func (s *OAuthStore) Cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	for key, pa := range s.pendingAuthz {
		if now.Sub(pa.CreatedAt) > pendingAuthzTTL {
			delete(s.csrfTokens, pa.CSRFToken)
			delete(s.pendingAuthz, key)
		}
	}

	for key, ac := range s.authCodes {
		if now.Sub(ac.CreatedAt) > authCodeTTL {
			delete(s.authCodes, key)
		}
	}
}

func (s *OAuthStore) StartCleanupLoop(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.Cleanup()
			}
		}
	}()
}
