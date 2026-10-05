package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDashboardRejectsUntrustedBrowserOrigins(t *testing.T) {
	for _, origin := range []string{"https://attacker.example", "null", "http://localhost.attacker.example", "http://wails.localhost.attacker.example"} {
		for _, method := range []string{"GET", "POST", "OPTIONS"} {
			t.Run(origin+method, func(t *testing.T) {
				called := false
				h := corsLocalhost(localhostOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })))
				r := httptest.NewRequest(method, "http://127.0.0.1:8383/api/settings", nil)
				r.RemoteAddr = "127.0.0.1:1234"
				r.Header.Set("Origin", origin)
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if w.Code != http.StatusForbidden || called || w.Header().Get("Access-Control-Allow-Origin") != "" {
					t.Fatalf("untrusted origin accepted: status=%d handler=%v", w.Code, called)
				}
			})
		}
	}
}

func TestDashboardAllowsLocalClients(t *testing.T) {
	for _, origin := range []string{"", "http://wails.localhost", "wails://wails", "http://localhost:5173", "http://127.0.0.1:8383", "http://[::1]:5173"} {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8383/api/status", nil)
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		corsLocalhost(localhostOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))).ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("origin %q: status %d", origin, w.Code)
		}
	}
}

func TestWebSocketRejectsUntrustedBrowserOrigin(t *testing.T) {
	h := NewHub()
	r := httptest.NewRequest("GET", "http://127.0.0.1:8383/ws", nil)
	r.Header.Set("Origin", "https://attacker.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden || h.ClientCount() != 0 {
		t.Fatalf("untrusted websocket reached upgrade: status=%d", w.Code)
	}
}

func TestDashboardRejectsReboundHost(t *testing.T) {
	r := httptest.NewRequest("GET", "http://attacker.example/api/settings", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	localhostOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })).ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("rebound hostname accepted: %d", w.Code)
	}
}
