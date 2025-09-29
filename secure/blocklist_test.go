package secure

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

func TestTokenBlocklist_AddToken(t *testing.T) {
	tb := NewTokenBlocklist()
	token := "test-token-123"

	tb.AddToken(token)

	if !tb.IsBlocked(token) {
		t.Errorf("expected token to be blocked after adding")
	}
}

func TestTokenBlocklist_IsBlocked(t *testing.T) {
	tb := NewTokenBlocklist()
	blockedToken := "blocked-token"
	unblockedToken := "unblocked-token"

	tb.AddToken(blockedToken)

	if !tb.IsBlocked(blockedToken) {
		t.Errorf("expected blocked token to return true")
	}

	if tb.IsBlocked(unblockedToken) {
		t.Errorf("expected unblocked token to return false")
	}
}

func TestTokenBlocklist_RemoveToken(t *testing.T) {
	tb := NewTokenBlocklist()
	token := "test-token"

	tb.AddToken(token)
	if !tb.IsBlocked(token) {
		t.Errorf("expected token to be blocked after adding")
	}

	tb.RemoveToken(token)
	if tb.IsBlocked(token) {
		t.Errorf("expected token to not be blocked after removal")
	}
}

func TestTokenBlocklist_LoadTokensFromSlice(t *testing.T) {
	tb := NewTokenBlocklist()
	tokens := []string{
		"token1",
		"token2",
		"token3",
	}

	tb.LoadTokensFromSlice(tokens)

	if tb.Count() != 3 {
		t.Errorf("expected count to be 3, got %d", tb.Count())
	}

	for _, token := range tokens {
		if !tb.IsBlocked(token) {
			t.Errorf("expected token %s to be blocked", token)
		}
	}
}

func TestTokenBlocklist_LoadTokensFromSlice_Overwrites(t *testing.T) {
	tb := NewTokenBlocklist()

	// Add initial tokens
	tb.AddToken("old-token")
	if tb.Count() != 1 {
		t.Errorf("expected count to be 1, got %d", tb.Count())
	}

	// Load new tokens (should overwrite)
	newTokens := []string{"new-token1", "new-token2"}
	tb.LoadTokensFromSlice(newTokens)

	if tb.Count() != 2 {
		t.Errorf("expected count to be 2, got %d", tb.Count())
	}

	if tb.IsBlocked("old-token") {
		t.Errorf("old token should have been removed")
	}

	if !tb.IsBlocked("new-token1") || !tb.IsBlocked("new-token2") {
		t.Errorf("new tokens should be blocked")
	}
}

func TestTokenBlocklist_Count(t *testing.T) {
	tb := NewTokenBlocklist()

	if tb.Count() != 0 {
		t.Errorf("expected initial count to be 0, got %d", tb.Count())
	}

	tb.AddToken("token1")
	if tb.Count() != 1 {
		t.Errorf("expected count to be 1, got %d", tb.Count())
	}

	tb.AddToken("token2")
	if tb.Count() != 2 {
		t.Errorf("expected count to be 2, got %d", tb.Count())
	}

	tb.RemoveToken("token1")
	if tb.Count() != 1 {
		t.Errorf("expected count to be 1 after removal, got %d", tb.Count())
	}
}

func TestTokenBlocklist_ConcurrentAccess(t *testing.T) {
	tb := NewTokenBlocklist()
	var wg sync.WaitGroup
	numGoroutines := 100
	tokensPerGoroutine := 10

	// Concurrent adds
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < tokensPerGoroutine; j++ {
				token := string(rune('a' + (id*tokensPerGoroutine+j)%26))
				tb.AddToken(token)
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < tokensPerGoroutine; j++ {
				token := string(rune('a' + (id*tokensPerGoroutine+j)%26))
				tb.IsBlocked(token)
			}
		}(i)
	}

	wg.Wait()

	// Verify that operations completed without data races
	if tb.Count() < 1 {
		t.Errorf("expected at least 1 token after concurrent operations")
	}
}

func TestLoadBlockedTokensFromFile(t *testing.T) {
	// Create a temporary blocklist file
	tempDir := t.TempDir()
	blocklistPath := filepath.Join(tempDir, "blocklist.txt")

	content := `# This is a comment
token1
token2
# Another comment
token3

# Empty line above should be ignored
token4
`

	err := os.WriteFile(blocklistPath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Reset global blocklist before test
	globalTokenBlocklist = NewTokenBlocklist()

	// Load tokens from file
	err = LoadBlockedTokensFromFile(blocklistPath)
	if err != nil {
		t.Fatalf("LoadBlockedTokensFromFile failed: %v", err)
	}

	// Verify expected tokens are blocked
	expectedTokens := []string{"token1", "token2", "token3", "token4"}
	for _, token := range expectedTokens {
		if !globalTokenBlocklist.IsBlocked(token) {
			t.Errorf("expected token %s to be blocked", token)
		}
	}

	// Verify count
	if globalTokenBlocklist.Count() != 4 {
		t.Errorf("expected 4 tokens to be loaded, got %d", globalTokenBlocklist.Count())
	}
}

func TestLoadBlockedTokensFromFile_EmptyFile(t *testing.T) {
	tempDir := t.TempDir()
	blocklistPath := filepath.Join(tempDir, "empty.txt")

	err := os.WriteFile(blocklistPath, []byte(""), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Reset global blocklist before test
	globalTokenBlocklist = NewTokenBlocklist()

	err = LoadBlockedTokensFromFile(blocklistPath)
	if err != nil {
		t.Fatalf("LoadBlockedTokensFromFile failed: %v", err)
	}

	if globalTokenBlocklist.Count() != 0 {
		t.Errorf("expected 0 tokens for empty file, got %d", globalTokenBlocklist.Count())
	}
}

func TestLoadBlockedTokensFromFile_OnlyComments(t *testing.T) {
	tempDir := t.TempDir()
	blocklistPath := filepath.Join(tempDir, "comments.txt")

	content := `# Comment 1
# Comment 2
# Comment 3
`

	err := os.WriteFile(blocklistPath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Reset global blocklist before test
	globalTokenBlocklist = NewTokenBlocklist()

	err = LoadBlockedTokensFromFile(blocklistPath)
	if err != nil {
		t.Fatalf("LoadBlockedTokensFromFile failed: %v", err)
	}

	if globalTokenBlocklist.Count() != 0 {
		t.Errorf("expected 0 tokens for comments-only file, got %d", globalTokenBlocklist.Count())
	}
}

func TestLoadBlockedTokensFromFile_NonexistentFile(t *testing.T) {
	// Reset global blocklist before test
	globalTokenBlocklist = NewTokenBlocklist()

	err := LoadBlockedTokensFromFile("/nonexistent/path/blocklist.txt")
	if err == nil {
		t.Errorf("expected error for nonexistent file, got nil")
	}
}

func TestAddTokenToBlocklist(t *testing.T) {
	// Reset global blocklist before test
	globalTokenBlocklist = NewTokenBlocklist()

	token := "global-test-token"
	AddTokenToBlocklist(token)

	if !globalTokenBlocklist.IsBlocked(token) {
		t.Errorf("expected token to be blocked in global blocklist")
	}
}

func TestRemoveTokenFromBlocklist(t *testing.T) {
	// Reset global blocklist before test
	globalTokenBlocklist = NewTokenBlocklist()

	token := "remove-test-token"
	AddTokenToBlocklist(token)

	if !globalTokenBlocklist.IsBlocked(token) {
		t.Errorf("expected token to be blocked after adding")
	}

	RemoveTokenFromBlocklist(token)

	if globalTokenBlocklist.IsBlocked(token) {
		t.Errorf("expected token to not be blocked after removal")
	}
}

func TestLoadBlockedTokens(t *testing.T) {
	// Reset global blocklist before test
	globalTokenBlocklist = NewTokenBlocklist()

	tokens := []string{"token-a", "token-b", "token-c"}
	LoadBlockedTokens(tokens)

	if globalTokenBlocklist.Count() != 3 {
		t.Errorf("expected 3 tokens, got %d", globalTokenBlocklist.Count())
	}

	for _, token := range tokens {
		if !globalTokenBlocklist.IsBlocked(token) {
			t.Errorf("expected token %s to be blocked", token)
		}
	}
}

func TestGetTokenBlocklist(t *testing.T) {
	// Reset global blocklist before test
	globalTokenBlocklist = NewTokenBlocklist()

	tb := GetTokenBlocklist()
	if tb == nil {
		t.Errorf("expected GetTokenBlocklist to return non-nil blocklist")
	}

	// Verify it's the same instance
	token := "test-global"
	tb.AddToken(token)

	if !globalTokenBlocklist.IsBlocked(token) {
		t.Errorf("expected modification through GetTokenBlocklist to affect global instance")
	}
}

func TestLoadBlockedTokensFromFile_WithWhitespace(t *testing.T) {
	tempDir := t.TempDir()
	blocklistPath := filepath.Join(tempDir, "whitespace.txt")

	content := `  token-with-leading-space
token-with-trailing-space
  token-with-both
	token-with-tab
`

	err := os.WriteFile(blocklistPath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Reset global blocklist before test
	globalTokenBlocklist = NewTokenBlocklist()

	err = LoadBlockedTokensFromFile(blocklistPath)
	if err != nil {
		t.Fatalf("LoadBlockedTokensFromFile failed: %v", err)
	}

	// Verify tokens are trimmed properly
	expectedTokens := []string{
		"token-with-leading-space",
		"token-with-trailing-space",
		"token-with-both",
		"token-with-tab",
	}

	for _, token := range expectedTokens {
		if !globalTokenBlocklist.IsBlocked(token) {
			t.Errorf("expected token %s to be blocked", token)
		}
	}

	if globalTokenBlocklist.Count() != 4 {
		t.Errorf("expected 4 tokens, got %d", globalTokenBlocklist.Count())
	}
}

func TestAuthMiddleware_BlockedToken(t *testing.T) {
	// Create a test secret for JWT signing
	secret := []byte("test-secret-key-12345")
	JWTSecret = secret

	// Reset global blocklist
	globalTokenBlocklist = NewTokenBlocklist()

	// Create a valid JWT token
	claims := &jwtCustomClaims{
		Email:    "test@example.com",
		Level:    "admin",
		ImageURL: "https://example.com/image.jpg",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * 24)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(secret)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// Create Echo instance
	e := echo.New()

	// Create EchoSecure instance
	secureAPI := &EchoSecure{
		e:                  e,
		secret:             secret,
		enableAuthenticate: true,
		enableAuthorize:    false,
		manCert:            false,
		config:             SecureConfig{},
	}

	// Create a test handler
	testHandler := func(c echo.Context) error {
		return c.String(http.StatusOK, "success")
	}

	// Test 1: Request with valid non-blocked token should succeed
	t.Run("ValidNonBlockedToken", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+tokenString)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := secureAPI.AuthMiddleware(NOAUTH)(testHandler)
		err := handler(c)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}

		if rec.Body.String() != "success" {
			t.Errorf("expected body 'success', got %s", rec.Body.String())
		}
	})

	// Test 2: Block the token and verify request is rejected
	t.Run("BlockedToken", func(t *testing.T) {
		// Add token to blocklist
		AddTokenToBlocklist(tokenString)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+tokenString)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := secureAPI.AuthMiddleware(NOAUTH)(testHandler)
		err := handler(c)

		if err == nil {
			t.Fatalf("expected error for blocked token, got nil")
		}

		httpErr, ok := err.(*echo.HTTPError)
		if !ok {
			t.Fatalf("expected echo.HTTPError, got %T", err)
		}

		if httpErr.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", httpErr.Code)
		}

		if httpErr.Message != "token has been revoked" {
			t.Errorf("expected message 'token has been revoked', got %v", httpErr.Message)
		}
	})

	// Test 3: Remove token from blocklist and verify request succeeds again
	t.Run("UnblockedToken", func(t *testing.T) {
		// Remove token from blocklist
		RemoveTokenFromBlocklist(tokenString)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+tokenString)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := secureAPI.AuthMiddleware(NOAUTH)(testHandler)
		err := handler(c)

		if err != nil {
			t.Errorf("expected no error after unblocking, got %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})
}

func TestAuthMiddleware_BlockedTokenFromFile(t *testing.T) {
	// Create a test secret for JWT signing
	secret := []byte("test-secret-key-for-file")
	JWTSecret = secret

	// Create a valid JWT token
	claims := &jwtCustomClaims{
		Email:    "filetest@example.com",
		Level:    "user",
		ImageURL: "https://example.com/filetest.jpg",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * 24)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(secret)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// Create a temporary blocklist file with the token
	tempDir := t.TempDir()
	blocklistPath := filepath.Join(tempDir, "blocklist.txt")

	content := "# Blocklist file for integration test\n" + tokenString + "\n"
	err = os.WriteFile(blocklistPath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to create blocklist file: %v", err)
	}

	// Reset and load blocklist from file
	globalTokenBlocklist = NewTokenBlocklist()
	err = LoadBlockedTokensFromFile(blocklistPath)
	if err != nil {
		t.Fatalf("failed to load blocklist: %v", err)
	}

	// Create Echo instance
	e := echo.New()

	// Create EchoSecure instance
	secureAPI := &EchoSecure{
		e:                  e,
		secret:             secret,
		enableAuthenticate: true,
		enableAuthorize:    false,
		manCert:            false,
		config: SecureConfig{
			TokenBlocklistFile: blocklistPath,
		},
	}

	// Create a test handler
	testHandler := func(c echo.Context) error {
		return c.String(http.StatusOK, "success")
	}

	// Make request with blocked token
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+tokenString)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := secureAPI.AuthMiddleware(NOAUTH)(testHandler)
	err = handler(c)

	if err == nil {
		t.Fatalf("expected error for blocked token loaded from file, got nil")
	}

	httpErr, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected echo.HTTPError, got %T", err)
	}

	if httpErr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", httpErr.Code)
	}

	if httpErr.Message != "token has been revoked" {
		t.Errorf("expected message 'token has been revoked', got %v", httpErr.Message)
	}
}