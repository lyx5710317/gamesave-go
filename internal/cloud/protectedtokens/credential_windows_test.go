//go:build windows

package protectedtokens

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestWindowsCredentialRoundTripRotationAndIsolation(t *testing.T) {
	installationID := "test_" + uuid.NewString()
	s, err := New("baidu_netdisk", installationID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Delete(); err != nil {
			t.Errorf("cleanup Windows test credential: %v", err)
		}
	})
	if _, err := s.Load(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing credential = %v, want ErrNotFound", err)
	}
	first := Tokens{AccessToken: "test-access-1", RefreshToken: "test-refresh-1", ExpiresAtUnixMs: time.Now().Add(time.Hour).UnixMilli()}
	if err := s.Save(first); err != nil {
		t.Fatal(err)
	}
	assertTokens(t, s, first)

	otherInstall, _ := New("baidu_netdisk", "other_"+uuid.NewString())
	if _, err := otherInstall.Load(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other installation read this credential: %v", err)
	}
	otherProvider, _ := New("dropbox", installationID)
	if _, err := otherProvider.Load(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other provider read this credential: %v", err)
	}

	rotated := Tokens{AccessToken: "test-access-2", RefreshToken: "test-refresh-2", ExpiresAtUnixMs: first.ExpiresAtUnixMs + 1000}
	if err := s.Save(rotated); err != nil {
		t.Fatal(err)
	}
	assertTokens(t, s, rotated)
	oversized := rotated
	oversized.RefreshToken = strings.Repeat("x", maxCredentialBlobBytes)
	if err := s.Save(oversized); err == nil {
		t.Fatal("oversized token record was accepted")
	}
	assertTokens(t, s, rotated)

	if err := s.Delete(); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(); err != nil {
		t.Fatalf("second Delete: %v", err)
	}
	if _, err := s.Load(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted credential = %v, want ErrNotFound", err)
	}
}

func TestWindowsCredentialCorruptionFailsClosed(t *testing.T) {
	s, err := New("baidu_netdisk", "test_"+uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Delete() })
	if err := writeCredential(s.target, []byte("not-json")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("corrupt credential = %v", err)
	}
	if err := writeCredential(s.target, []byte(`{"version":1,"provider":"other","tokens":{"accessToken":"a","refreshToken":"r","expiresAtUnixMs":1}}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(); err == nil {
		t.Fatal("accepted a credential tagged for another provider")
	}
}

func assertTokens(t *testing.T, s *Store, want Tokens) {
	t.Helper()
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatal("loaded tokens differ from the saved token pair")
	}
}
