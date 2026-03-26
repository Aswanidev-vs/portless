package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestHashPasswordUsesBcryptAndSupportsLegacyVerification(t *testing.T) {
	hash := HashPassword("secret-password")
	if hash == "" {
		t.Fatal("expected bcrypt hash")
	}
	if !VerifyPassword("secret-password", hash) {
		t.Fatal("expected bcrypt verification to pass")
	}
	if VerifyPassword("wrong-password", hash) {
		t.Fatal("expected bcrypt verification to fail for wrong password")
	}

	legacy := "95d30169a59c418b52013316d5400beec7db286902b866b10b7c2f7096617f41"
	if !VerifyPassword("secret-password", legacy) {
		t.Fatal("expected legacy sha256 verification to pass")
	}
}

func TestTOTPFlowAndBackupCode(t *testing.T) {
	store := NewMemoryStore()
	authenticator := NewAuthenticator(Config{Store: store})
	ctx := context.Background()

	user, err := authenticator.CreateUser(ctx, CreateUserRequest{
		Username:    "alice",
		Email:       "alice@example.com",
		Password:    "secret-password",
		WorkspaceID: "workspace-1",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	session, err := authenticator.AuthenticateWithPassword(ctx, "alice", "secret-password")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if session.MFAVerified {
		t.Fatal("expected MFA verification to be pending before MFA is enabled")
	}

	setup, err := authenticator.EnableMFA(ctx, user.ID, "totp")
	if err != nil {
		t.Fatalf("enable MFA: %v", err)
	}
	if len(setup.BackupCodes) == 0 {
		t.Fatal("expected backup codes")
	}

	session, err = authenticator.AuthenticateWithPassword(ctx, "alice", "secret-password")
	if err != nil {
		t.Fatalf("authenticate after MFA enable: %v", err)
	}
	if session.MFAVerified {
		t.Fatal("expected MFA verification to remain false until TOTP/backup verification")
	}

	code := generateTOTPCodeForCurrentWindow(setup.Secret)
	if err := authenticator.VerifyMFA(ctx, session.Token, code); err != nil {
		t.Fatalf("verify MFA with TOTP: %v", err)
	}

	verifiedSession, err := store.GetSession(ctx, session.Token)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if !verifiedSession.MFAVerified {
		t.Fatal("expected session to be MFA verified")
	}

	session, err = authenticator.AuthenticateWithPassword(ctx, "alice", "secret-password")
	if err != nil {
		t.Fatalf("authenticate for backup code: %v", err)
	}
	backupCode := setup.BackupCodes[0]
	if err := authenticator.VerifyMFA(ctx, session.Token, backupCode); err != nil {
		t.Fatalf("verify MFA with backup code: %v", err)
	}

	updatedUser, err := store.GetUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("load user: %v", err)
	}
	for _, code := range updatedUser.MFA.BackupCodes {
		if code == backupCode {
			t.Fatal("expected used backup code to be removed")
		}
	}
}

func generateTOTPCodeForCurrentWindow(secret string) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(normalizeTOTPSecret(secret))
	if err != nil {
		return ""
	}
	return generateTOTPCode(key, time.Now().Unix()/30)
}

func TestStaticJWTSSOProvider(t *testing.T) {
	provider := NewStaticJWTSSOProvider("oidc-hs256", "https://issuer.example", "gotunnel-dashboard", "super-secret")
	token := signTestJWT(t, map[string]interface{}{
		"iss":                "https://issuer.example",
		"aud":                "gotunnel-dashboard",
		"sub":                "user-123",
		"email":              "alice@example.com",
		"name":               "Alice",
		"preferred_username": "alice",
		"groups":             []string{"developers", "operators"},
		"exp":                time.Now().Add(5 * time.Minute).Unix(),
	}, "super-secret")

	user, err := provider.ValidateToken(context.Background(), token)
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}
	if user.Username != "alice" {
		t.Fatalf("username = %q, want alice", user.Username)
	}
	if len(user.Groups) != 2 {
		t.Fatalf("groups = %v, want 2 groups", user.Groups)
	}
}

func signTestJWT(t *testing.T, claims map[string]interface{}, secret string) string {
	t.Helper()
	headerJSON, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	header := base64.RawURLEncoding.EncodeToString(headerJSON)
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(header + "." + payload))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return strings.Join([]string{header, payload, signature}, ".")
}
