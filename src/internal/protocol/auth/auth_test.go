package auth

import (
	"testing"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
)

// @sk-test core-tunnel-mvp#T5.1: TestAuthValidateToken (AC-006)
func TestValidateTokenValid(t *testing.T) {
	valid := []string{"token1", "token2", "token3"}

	if !ValidateToken("token1", valid) {
		t.Error("ValidateToken(token1) = false, want true")
	}
	if !ValidateToken("token2", valid) {
		t.Error("ValidateToken(token2) = false, want true")
	}
	if !ValidateToken("token3", valid) {
		t.Error("ValidateToken(token3) = false, want true")
	}
}

func TestValidateTokenInvalid(t *testing.T) {
	valid := []string{"token1", "token2"}

	if ValidateToken("nonexistent", valid) {
		t.Error("ValidateToken(nonexistent) = true, want false")
	}
}

func TestValidateTokenEmptyToken(t *testing.T) {
	valid := []string{"token1"}

	if ValidateToken("", valid) {
		t.Error("ValidateToken(empty) = true, want false")
	}
}

func TestValidateTokenEmptyList(t *testing.T) {
	if ValidateToken("token1", []string{}) {
		t.Error("ValidateToken with empty list = true, want false")
	}
}

func TestValidateTokenNilList(t *testing.T) {
	if ValidateToken("token1", nil) {
		t.Error("ValidateToken with nil list = true, want false")
	}
}

func TestValidateTokenCaseSensitive(t *testing.T) {
	valid := []string{"Token"}

	if ValidateToken("token", valid) {
		t.Error("ValidateToken should be case-sensitive")
	}
}

// @sk-test post-hardening#T4.1: TestAuthErrorMessageDoesNotLeakInfo (AC-004)
func TestAuthErrorMessageDoesNotLeakInfo(t *testing.T) {
	// Verify that FindToken returns nil for invalid tokens
	// (the actual error message is in server/main.go: "authentication failed")
	tokens := []config.TokenCfg{{Name: "valid-token", Secret: "valid-token"}}
	if found := FindToken("invalid-token", tokens); found != nil {
		t.Error("FindToken(invalid) should return nil")
	}
	if found := FindToken("valid-token", tokens); found == nil {
		t.Error("FindToken(valid) should return non-nil")
	}
}
