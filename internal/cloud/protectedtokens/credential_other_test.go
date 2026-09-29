//go:build !windows

package protectedtokens

import (
	"errors"
	"testing"
)

func TestUnsupportedPlatformDoesNotUsePlaintextFallback(t *testing.T) {
	s, err := New("baidu_netdisk", "node_123")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save(Tokens{AccessToken: "dummy", RefreshToken: "dummy", ExpiresAtUnixMs: 1}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Save = %v, want ErrUnsupported", err)
	}
	if _, err := s.Load(); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Load = %v, want ErrUnsupported", err)
	}
	if err := s.Delete(); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Delete = %v, want ErrUnsupported", err)
	}
}
