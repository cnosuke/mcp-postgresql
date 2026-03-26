package server

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/modelcontextprotocol/go-sdk/auth"
)

type JWTManager struct {
	signingKey  []byte
	issuer      string
	tokenExpiry time.Duration
}

func NewJWTManager(signingKey, issuer string, tokenExpirySec int) *JWTManager {
	return &JWTManager{
		signingKey:  []byte(signingKey),
		issuer:      issuer,
		tokenExpiry: time.Duration(tokenExpirySec) * time.Second,
	}
}

type AccessTokenClaims struct {
	jwt.RegisteredClaims
	Email    string `json:"email"`
	Scope    string `json:"scope"`
	ClientID string `json:"client_id"`
}

func (m *JWTManager) IssueAccessToken(userID, email, scope, audience, clientID string) (string, error) {
	now := time.Now()
	claims := AccessTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Audience:  jwt.ClaimStrings{audience},
			Issuer:    m.issuer,
			ExpiresAt: jwt.NewNumericDate(now.Add(m.tokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Email:    email,
		Scope:    scope,
		ClientID: clientID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.signingKey)
}

func (m *JWTManager) VerifyAccessToken(tokenString, expectedAudience string) (*AccessTokenClaims, error) {
	claims := &AccessTokenClaims{}
	_, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		return m.signingKey, nil
	}, jwt.WithIssuer(m.issuer))
	if err != nil {
		return nil, err
	}

	if !slices.Contains([]string(claims.Audience), expectedAudience) {
		return nil, fmt.Errorf("token audience does not contain %q", expectedAudience)
	}

	return claims, nil
}

func (m *JWTManager) MakeTokenVerifier(expectedAudience string) auth.TokenVerifier {
	return func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		claims, err := m.VerifyAccessToken(token, expectedAudience)
		if err != nil {
			return nil, fmt.Errorf("access token verification failed: %w", auth.ErrInvalidToken)
		}

		return &auth.TokenInfo{
			UserID:     claims.Subject,
			Scopes:     strings.Fields(claims.Scope),
			Expiration: claims.ExpiresAt.Time,
			Extra: map[string]any{
				"email":     claims.Email,
				"client_id": claims.ClientID,
			},
		}, nil
	}
}
