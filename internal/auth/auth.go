package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// TokenManager handles authentication token validation and rotation
type TokenManager struct {
	mu          sync.RWMutex
	tokens      map[string]struct{} // base64 encoded tokens
	tokenFile   string
	logger      *zap.Logger
	stopWatch   chan struct{}
	wg          sync.WaitGroup
}

// NewTokenManager creates a new token manager
func NewTokenManager(tokens []string, tokenFile string, logger *zap.Logger) (*TokenManager, error) {
	tm := &TokenManager{
		tokens:    make(map[string]struct{}),
		tokenFile: tokenFile,
		logger:    logger,
		stopWatch: make(chan struct{}),
	}

	// Load initial tokens
	for _, tok := range tokens {
		tm.tokens[tok] = struct{}{}
	}

	// Load from file if specified
	if tokenFile != "" {
		if err := tm.loadFromFile(); err != nil {
			return nil, err
		}
		// Start file watcher
		tm.wg.Add(1)
		go tm.watchFile()
	}

	return tm, nil
}

// ValidateToken checks if a token is valid (constant-time comparison)
func (tm *TokenManager) ValidateToken(token string) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	// Check against all valid tokens using constant-time comparison
	for validToken := range tm.tokens {
		if subtle.ConstantTimeCompare([]byte(token), []byte(validToken)) == 1 {
			return true
		}
	}
	return false
}

// ValidateTokenBytes validates a raw token byte slice
func (tm *TokenManager) ValidateTokenBytes(token []byte) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	for validToken := range tm.tokens {
		validBytes := []byte(validToken)
		if len(token) == len(validBytes) && subtle.ConstantTimeCompare(token, validBytes) == 1 {
			return true
		}
	}
	return false
}

// AddToken adds a new valid token
func (tm *TokenManager) AddToken(token string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.tokens[token] = struct{}{}
}

// RemoveToken removes a token
func (tm *TokenManager) RemoveToken(token string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	delete(tm.tokens, token)
}

// RotateTokens replaces all tokens with new ones
func (tm *TokenManager) RotateTokens(newTokens []string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.tokens = make(map[string]struct{}, len(newTokens))
	for _, tok := range newTokens {
		tm.tokens[tok] = struct{}{}
	}
	if tm.logger != nil {
		tm.logger.Info("Tokens rotated", zap.Int("count", len(newTokens)))
	}
}

// TokenCount returns the number of valid tokens
func (tm *TokenManager) TokenCount() int {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return len(tm.tokens)
}

// Close stops the file watcher
func (tm *TokenManager) Close() error {
	close(tm.stopWatch)
	tm.wg.Wait()
	return nil
}

func (tm *TokenManager) loadFromFile() error {
	data, err := os.ReadFile(tm.tokenFile)
	if err != nil {
		if os.IsNotExist(err) {
			tm.logger.Warn("Token file not found", zap.String("file", tm.tokenFile))
			return nil
		}
		return err
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	lines := string(data)
	count := 0
	for _, line := range splitLines(lines) {
		line = trimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		// Validate base64
		if _, err := base64.StdEncoding.DecodeString(line); err != nil {
			tm.logger.Warn("Invalid token in file", zap.String("file", tm.tokenFile), zap.Error(err))
			continue
		}
		tm.tokens[line] = struct{}{}
		count++
	}

	tm.logger.Info("Loaded tokens from file", zap.String("file", tm.tokenFile), zap.Int("count", count))
	return nil
}

func (tm *TokenManager) watchFile() {
	defer tm.wg.Done()
	var lastMod time.Time
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-tm.stopWatch:
			return
		case <-ticker.C:
			info, err := os.Stat(tm.tokenFile)
			if err != nil {
				if !os.IsNotExist(err) {
					tm.logger.Error("Token file stat error", zap.Error(err))
				}
				continue
			}
			if info.ModTime().After(lastMod) {
				lastMod = info.ModTime()
				if err := tm.loadFromFile(); err != nil {
					tm.logger.Error("Token file reload failed", zap.Error(err))
				} else {
					tm.logger.Info("Tokens reloaded from file", zap.String("file", tm.tokenFile))
				}
			}
		}
	}
}

// GenerateToken generates a new cryptographically secure token
func GenerateToken() (string, error) {
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(token), nil
}

// GenerateTokenBytes generates a new token as byte slice
func GenerateTokenBytes() ([]byte, error) {
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return nil, err
	}
	return token, nil
}

// DecodeToken decodes a base64 token
func DecodeToken(token string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(token)
}

// EncodeToken encodes a token to base64
func EncodeToken(token []byte) string {
	return base64.StdEncoding.EncodeToString(token)
}

// CompareTokens compares two tokens in constant time
func CompareTokens(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

var (
	ErrInvalidTokenFormat = errors.New("invalid token format (not base64)")
	ErrTokenFileNotFound  = errors.New("token file not found")
)

// ValidateTokenFormat validates a token is valid base64
func ValidateTokenFormat(token string) error {
	if token == "" {
		return ErrInvalidTokenFormat
	}
	if _, err := base64.StdEncoding.DecodeString(token); err != nil {
		return ErrInvalidTokenFormat
	}
	return nil
}

// LoadTokensFromFile loads tokens from a file
func LoadTokensFromFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tokens []string
	for _, line := range splitLines(string(data)) {
		line = trimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		if err := ValidateTokenFormat(line); err != nil {
			return nil, fmt.Errorf("invalid token in %s: %w", path, err)
		}
		tokens = append(tokens, line)
	}
	return tokens, nil
}

// SaveTokensToFile saves tokens to a file (atomic write)
func SaveTokensToFile(path string, tokens []string) error {
	dir := filepath.Dir(path)
	tmpFile := filepath.Join(dir, ".tokens.tmp")
	
	var content string
	for _, tok := range tokens {
		content += tok + "\n"
	}
	
	if err := os.WriteFile(tmpFile, []byte(content), 0600); err != nil {
		return err
	}
	return os.Rename(tmpFile, path)
}

// Helper functions
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			// Handle Windows CRLF
			line = strings.TrimSuffix(line, "\r")
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(s) {
		line := s[start:]
		line = strings.TrimSuffix(line, "\r")
		lines = append(lines, line)
	}
	return lines
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}