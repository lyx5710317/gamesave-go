package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCallbackPageEscapesUntrustedDetails(t *testing.T) {
	for _, success := range []bool{true, false} {
		w := httptest.NewRecorder()
		writeCallbackPage(w, success, `<script>alert("test")</script><img src=x onerror=alert(1)>`)
		body := w.Body.String()
		if strings.Contains(body, "<script>") || strings.Contains(body, "<img src=x") || !strings.Contains(body, "&lt;script&gt;") {
			t.Fatalf("callback rendered executable markup: %s", body)
		}
	}
}

func TestAuthStartBindsBrowserCallbackToFreshState(t *testing.T) {
	ts := startTestServer(t)
	t.Cleanup(func() {
		activeAuthListener.stop()
		pendingPKCE.Lock()
		pendingPKCE.provider, pendingPKCE.verifier, pendingPKCE.state = "", "", ""
		pendingPKCE.Unlock()
	})
	resp, body := ts.do(t, http.MethodPost, "/api/auth/start", map[string]string{"provider": "google_drive"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("start auth: %d", resp.StatusCode)
	}
	var authURL string
	if err := json.Unmarshal(body["authUrl"], &authURL); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	state := u.Query().Get("state")
	pendingPKCE.Lock()
	want := pendingPKCE.state
	pendingPKCE.Unlock()
	if state == "" || state != want || u.Query().Get("code_challenge") == "" {
		t.Fatal("authorization URL is not bound to pending PKCE state")
	}
	w := httptest.NewRecorder()
	ts.server.handleBrowserCallback(w, httptest.NewRequest("GET", "http://localhost/callback?error=access_denied&state="+url.QueryEscape(state), nil))
	pendingPKCE.Lock()
	remaining := pendingPKCE.state
	pendingPKCE.Unlock()
	if remaining != "" {
		t.Fatal("matching callback did not consume state")
	}
	w = httptest.NewRecorder()
	ts.server.handleBrowserCallback(w, httptest.NewRequest("GET", "http://localhost/callback?error=access_denied&state="+url.QueryEscape(state), nil))
	if w.Code != http.StatusBadRequest {
		t.Fatal("replayed callback accepted")
	}
}

func TestBrowserCallbackRejectsUnmatchedStateWithoutConsumingFlow(t *testing.T) {
	pendingPKCE.Lock()
	pendingPKCE.provider, pendingPKCE.verifier, pendingPKCE.state = "test", "synthetic-verifier", "expected-state"
	pendingPKCE.Unlock()
	t.Cleanup(func() {
		pendingPKCE.Lock()
		pendingPKCE.provider, pendingPKCE.verifier = "", ""
		pendingPKCE.state = ""
		pendingPKCE.Unlock()
	})
	s := &Server{Hub: NewHub()}
	r := httptest.NewRequest("GET", "http://localhost/callback?error=access_denied&state=unmatched", nil)
	w := httptest.NewRecorder()
	s.handleBrowserCallback(w, r)
	pendingPKCE.Lock()
	defer pendingPKCE.Unlock()
	if pendingPKCE.provider != "test" || pendingPKCE.verifier != "synthetic-verifier" {
		t.Fatal("unmatched callback consumed the pending authorization")
	}
}

func TestOldAuthAttemptCannotStopReplacementListener(t *testing.T) {
	srv := &http.Server{}
	l := &authListener{servers: []*http.Server{srv}, state: "replacement"}
	l.stopState("expired-attempt")
	if len(l.servers) != 1 || l.servers[0] != srv || l.done {
		t.Fatal("old attempt stopped the replacement sign-in")
	}
	l.stopState("replacement")
	if len(l.servers) != 0 || !l.done {
		t.Fatal("matching attempt was not stopped")
	}
}
