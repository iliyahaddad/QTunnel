package auth

import (
	"testing"
)

func TestGenerateToken(t *testing.T) {
	token, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}
	if token == "" {
		t.Error("GenerateToken returned empty string")
	}
	
	// Validate it's valid base64
	if err := ValidateTokenFormat(token); err != nil {
		t.Errorf("Generated token is not valid base64: %v", err)
	}
}

func TestValidateToken(t *testing.T) {
	tokenMgr, _ := NewTokenManager([]string{"dGVzdC10b2tlbi10aGF0LWlzLTMtMTEteHh4eHh4eHg="}, "", nil)
	
	if !tokenMgr.ValidateToken("dGVzdC10b2tlbi10aGF0LWlzLTMtMTEteHh4eHh4eHg=") {
		t.Error("ValidateToken should return true for valid token")
	}
	
	if tokenMgr.ValidateToken("invalid-token") {
		t.Error("ValidateToken should return false for invalid token")
	}
}

func TestConstantTimeCompare(t *testing.T) {
	token1 := "dGVzdC10b2tlbi10aGF0LWlzLTMtMTEteHh4eHh4eHg="
	token2 := "dGVzdC10b2tlbi10aGF0LWlzLTMtMTEteHh4eHh4eHg="
	token3 := "dGVzdC10b2tlbi10aGF0LWlzLTMtMTEteHh4eHh4eHg"
	
	if !CompareTokens(token1, token2) {
		t.Error("CompareTokens should return true for equal tokens")
	}
	
	if CompareTokens(token1, token3) {
		t.Error("CompareTokens should return false for different tokens")
	}
}

func TestTokenRotation(t *testing.T) {
	tokenMgr, _ := NewTokenManager([]string{"token1", "token2"}, "", nil)
	
	if tokenMgr.TokenCount() != 2 {
		t.Errorf("TokenCount = %d, want 2", tokenMgr.TokenCount())
	}
	
	tokenMgr.RotateTokens([]string{"newtoken1"})
	
	if tokenMgr.TokenCount() != 1 {
		t.Errorf("TokenCount after rotation = %d, want 1", tokenMgr.TokenCount())
	}
	
	if !tokenMgr.ValidateToken("newtoken1") {
		t.Error("New token should be valid after rotation")
	}
	
	if tokenMgr.ValidateToken("token1") {
		t.Error("Old token should not be valid after rotation")
	}
}

func TestAddRemoveToken(t *testing.T) {
	tokenMgr, _ := NewTokenManager([]string{}, "", nil)
	
	tokenMgr.AddToken("newtoken")
	
	if tokenMgr.TokenCount() != 1 {
		t.Errorf("TokenCount = %d, want 1", tokenMgr.TokenCount())
	}
	
	if !tokenMgr.ValidateToken("newtoken") {
		t.Error("Added token should be valid")
	}
	
	tokenMgr.RemoveToken("newtoken")
	
	if tokenMgr.TokenCount() != 0 {
		t.Errorf("TokenCount after remove = %d, want 0", tokenMgr.TokenCount())
	}
}