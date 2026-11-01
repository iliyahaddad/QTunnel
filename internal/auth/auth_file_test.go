package auth

import (
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestTokenFileWatch(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	tmpDir := t.TempDir()
	tokenFile := filepath.Join(tmpDir, "tokens.txt")

	// Write initial tokens
	initialTokens := []string{
		"dGVzdC10b2tlbi10aGF0LWlzLTMtMTEteHh4eHh4eHg=",   // test-token-that-is-31-xxxxxxxx
		"dGVzdC10b2tlbi10aGF0LWlzLTMtMTEteHh4eHh4eHgx",  // test-token-that-is-31-xxxxxxy
	}
	if err := SaveTokensToFile(tokenFile, initialTokens); err != nil {
		t.Fatalf("SaveTokensToFile failed: %v", err)
	}

	tm, err := NewTokenManager([]string{}, tokenFile, logger)
	if err != nil {
		t.Fatalf("NewTokenManager failed: %v", err)
	}
	defer tm.Close()

	// Should have loaded 2 tokens
	if tm.TokenCount() != 2 {
		t.Errorf("TokenCount = %d, want 2", tm.TokenCount())
	}

	// Add new token to file
	newTokens := append(initialTokens, "bmV3LXRva2VuLXRoYXQtaXMtMzEtMTEteHh4eHh4eHh5") // new-token-that-is-31-xxxxxxy
	if err := SaveTokensToFile(tokenFile, newTokens); err != nil {
		t.Fatalf("SaveTokensToFile failed: %v", err)
	}

	// Wait for file watcher to pick up changes
	time.Sleep(20 * time.Second)

	if tm.TokenCount() != 3 {
		t.Errorf("TokenCount after reload = %d, want 3", tm.TokenCount())
	}

	if !tm.ValidateToken("bmV3LXRva2VuLXRoYXQtaXMtMzEtMTEteHh4eHh4eHh5") {
		t.Error("New token should be valid")
	}
}

func TestTokenFileNotFound(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	tm, err := NewTokenManager([]string{}, "/nonexistent/tokens.txt", logger)
	if err != nil {
		t.Fatalf("NewTokenManager failed: %v", err)
	}
	defer tm.Close()

	// Should work with empty tokens
	if tm.TokenCount() != 0 {
		t.Errorf("TokenCount = %d, want 0", tm.TokenCount())
	}
}

func TestLoadTokensFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	tokenFile := filepath.Join(tmpDir, "tokens.txt")

	tokens := []string{
		"dGVzdC10b2tlbi10aGF0LWlzLTMtMTEteHh4eHh4eHg=",
		"dGVzdC10b2tlbi10aGF0LWlzLTMtMTEteHh4eHh4eHg=",
	}
	if err := SaveTokensToFile(tokenFile, tokens); err != nil {
		t.Fatalf("SaveTokensToFile failed: %v", err)
	}

	loaded, err := LoadTokensFromFile(tokenFile)
	if err != nil {
		t.Fatalf("LoadTokensFromFile failed: %v", err)
	}

	if len(loaded) != 2 {
		t.Errorf("Loaded %d tokens, want 2", len(loaded))
	}
}

func TestValidateTokenFormat_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{"valid base64", "dGVzdA==", false},
		{"valid base64 with padding", "YQ==", false},
		{"invalid char", "dGVzdA!", true},
		{"empty", "", true},
		{"not base64", "not-base64", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTokenFormat(tt.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTokenFormat(%q) error = %v, wantErr %v", tt.token, err, tt.wantErr)
			}
		})
	}
}