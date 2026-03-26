package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type StaticJWTSSOProvider struct {
	name     string
	issuer   string
	audience string
	secret   []byte
}

type jwtClaims struct {
	Subject           string   `json:"sub"`
	Email             string   `json:"email"`
	Name              string   `json:"name"`
	PreferredUsername string   `json:"preferred_username"`
	Groups            []string `json:"groups"`
	Issuer            string   `json:"iss"`
	Audience          any      `json:"aud"`
	ExpiresAt         int64    `json:"exp"`
	IssuedAt          int64    `json:"iat"`
}

func NewStaticJWTSSOProvider(name, issuer, audience, secret string) *StaticJWTSSOProvider {
	if strings.TrimSpace(name) == "" {
		name = "oidc-hs256"
	}
	return &StaticJWTSSOProvider{
		name:     name,
		issuer:   strings.TrimSpace(issuer),
		audience: strings.TrimSpace(audience),
		secret:   []byte(strings.TrimSpace(secret)),
	}
}

func (p *StaticJWTSSOProvider) Name() string { return p.name }

func (p *StaticJWTSSOProvider) GetAuthorizationURL(state string) string {
	return "https://example.invalid/" + p.name + "?state=" + state
}

func (p *StaticJWTSSOProvider) ExchangeCode(ctx context.Context, code string) (SSOUser, error) {
	return p.ValidateToken(ctx, code)
}

func (p *StaticJWTSSOProvider) ValidateToken(_ context.Context, token string) (SSOUser, error) {
	if len(p.secret) == 0 {
		return SSOUser{}, fmt.Errorf("jwt sso provider secret is not configured")
	}
	claims, err := parseAndValidateJWT(token, p.secret, p.issuer, p.audience)
	if err != nil {
		return SSOUser{}, err
	}
	username := firstNonEmptyJWT(claims.PreferredUsername, claims.Email, claims.Subject)
	if username == "" {
		return SSOUser{}, fmt.Errorf("jwt token missing username subject")
	}
	email := strings.TrimSpace(claims.Email)
	if email == "" {
		email = username + "@gotunnel.local"
	}
	displayName := firstNonEmptyJWT(claims.Name, username)
	return SSOUser{
		ExternalID:  firstNonEmptyJWT(claims.Subject, username),
		Email:       email,
		DisplayName: displayName,
		Username:    username,
		Groups:      append([]string(nil), claims.Groups...),
		Metadata: map[string]string{
			"issuer": p.issuer,
		},
	}, nil
}

func parseAndValidateJWT(token string, secret []byte, issuer, audience string) (jwtClaims, error) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 {
		return jwtClaims{}, fmt.Errorf("invalid jwt format")
	}

	headerPayload := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(headerPayload))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return jwtClaims{}, fmt.Errorf("invalid jwt signature")
	}

	var header struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
	}
	if err := decodeJWTPart(parts[0], &header); err != nil {
		return jwtClaims{}, fmt.Errorf("decode jwt header: %w", err)
	}
	if header.Algorithm != "HS256" {
		return jwtClaims{}, fmt.Errorf("unsupported jwt alg %q", header.Algorithm)
	}

	var claims jwtClaims
	if err := decodeJWTPart(parts[1], &claims); err != nil {
		return jwtClaims{}, fmt.Errorf("decode jwt claims: %w", err)
	}
	if issuer != "" && claims.Issuer != issuer {
		return jwtClaims{}, fmt.Errorf("unexpected issuer")
	}
	if audience != "" && !matchesAudience(claims.Audience, audience) {
		return jwtClaims{}, fmt.Errorf("unexpected audience")
	}
	if claims.ExpiresAt > 0 && time.Now().Unix() >= claims.ExpiresAt {
		return jwtClaims{}, fmt.Errorf("jwt token expired")
	}
	return claims, nil
}

func decodeJWTPart(part string, dst interface{}) error {
	payload, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, dst)
}

func matchesAudience(raw any, audience string) bool {
	switch value := raw.(type) {
	case string:
		return value == audience
	case []interface{}:
		for _, item := range value {
			if itemStr, ok := item.(string); ok && itemStr == audience {
				return true
			}
		}
	}
	return false
}

func firstNonEmptyJWT(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
