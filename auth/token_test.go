package auth

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestSaveAndLoadTokens(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tokens.json")

	store := NewTokenStore(path)

	token := TokenData{
		AccessToken:  "access-abc",
		RefreshToken: "refresh-xyz",
		ExpiresAt:    time.Now().Add(1 * time.Hour).Truncate(time.Second),
	}

	if err := store.Save("dev", token); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, err := store.TokenForSystem("dev")
	if err != nil {
		t.Fatalf("TokenForSystem failed: %v", err)
	}

	if got.AccessToken != token.AccessToken {
		t.Errorf("AccessToken: got %q, want %q", got.AccessToken, token.AccessToken)
	}
	if got.RefreshToken != token.RefreshToken {
		t.Errorf("RefreshToken: got %q, want %q", got.RefreshToken, token.RefreshToken)
	}
	if !got.ExpiresAt.Equal(token.ExpiresAt) {
		t.Errorf("ExpiresAt: got %v, want %v", got.ExpiresAt, token.ExpiresAt)
	}
}

func TestTokenForSystemNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tokens.json")

	store := NewTokenStore(path)

	_, err := store.TokenForSystem("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent system, got nil")
	}
}

func TestTokenFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission check not applicable on Windows")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "tokens.json")

	store := NewTokenStore(path)
	token := TokenData{
		AccessToken:  "tok",
		RefreshToken: "ref",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
	}

	if err := store.Save("sys", token); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("file permissions: got %o, want %o", perm, 0600)
	}
}

func TestSaveMultipleSystems(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tokens.json")

	store := NewTokenStore(path)

	devToken := TokenData{
		AccessToken:  "dev-access",
		RefreshToken: "dev-refresh",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
	}
	prodToken := TokenData{
		AccessToken:  "prod-access",
		RefreshToken: "prod-refresh",
		ExpiresAt:    time.Now().Add(2 * time.Hour),
	}

	if err := store.Save("dev", devToken); err != nil {
		t.Fatalf("Save dev failed: %v", err)
	}
	if err := store.Save("prod", prodToken); err != nil {
		t.Fatalf("Save prod failed: %v", err)
	}

	gotDev, err := store.TokenForSystem("dev")
	if err != nil {
		t.Fatalf("TokenForSystem dev failed: %v", err)
	}
	if gotDev.AccessToken != devToken.AccessToken {
		t.Errorf("dev AccessToken: got %q, want %q", gotDev.AccessToken, devToken.AccessToken)
	}

	gotProd, err := store.TokenForSystem("prod")
	if err != nil {
		t.Fatalf("TokenForSystem prod failed: %v", err)
	}
	if gotProd.AccessToken != prodToken.AccessToken {
		t.Errorf("prod AccessToken: got %q, want %q", gotProd.AccessToken, prodToken.AccessToken)
	}
}

func TestTokenIsExpired(t *testing.T) {
	expired := TokenData{
		AccessToken: "old",
		ExpiresAt:   time.Now().Add(-1 * time.Minute),
	}
	if !expired.IsExpired() {
		t.Error("expected expired token to return IsExpired() = true")
	}

	valid := TokenData{
		AccessToken: "new",
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	if valid.IsExpired() {
		t.Error("expected valid token to return IsExpired() = false")
	}
}

func TestLoadCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tokens.json")

	if err := os.WriteFile(path, []byte("not valid json {{{{"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	store := NewTokenStore(path)
	_, err := store.TokenForSystem("any")
	if err == nil {
		t.Fatal("expected error for corrupt JSON, got nil")
	}
}
