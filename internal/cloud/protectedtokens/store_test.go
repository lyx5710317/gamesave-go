package protectedtokens

import (
	"strings"
	"testing"
)

func TestNewRejectsInvalidCredentialIdentities(t *testing.T) {
	for _, pair := range [][2]string{
		{"", "node_1"}, {"baidu_netdisk", ""}, {"../baidu", "node_1"},
		{"baidu_netdisk", "node:1"}, {"baidu_netdisk", strings.Repeat("a", 129)},
	} {
		if _, err := New(pair[0], pair[1]); err == nil {
			t.Errorf("accepted invalid identity %q, %q", pair[0], pair[1])
		}
	}
	if _, err := New("baidu_netdisk", "node_123"); err != nil {
		t.Fatal(err)
	}
}

func TestSaveRejectsIncompleteTokensBeforeWriting(t *testing.T) {
	s, err := New("baidu_netdisk", "node_123")
	if err != nil {
		t.Fatal(err)
	}
	for _, tokens := range []Tokens{
		{}, {AccessToken: "access", RefreshToken: "refresh"},
		{AccessToken: "", RefreshToken: "refresh", ExpiresAtUnixMs: 1},
		{AccessToken: "access", RefreshToken: " ", ExpiresAtUnixMs: 1},
	} {
		if err := s.Save(tokens); err == nil {
			t.Fatal("accepted incomplete tokens")
		}
	}
}
