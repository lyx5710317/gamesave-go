package cloud

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/store"
)

func newTestService(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "opensave.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.EnsureDefaultSettings(t.TempDir(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	svc := New(s, func(level, msg string) {})
	return svc, s
}

func setCloudConfig(t *testing.T, s *store.Store, mutate func(*store.CloudConfig)) {
	t.Helper()
	cfg, err := s.GetCloudConfig()
	if err != nil {
		t.Fatal(err)
	}
	mutate(&cfg)
	if err := s.UpdateCloudConfig(cfg); err != nil {
		t.Fatal(err)
	}
}

func writeTempZip(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "snap.zip")
	if err := os.WriteFile(p, []byte(content), 0o666); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLocalFolderRoundTrip(t *testing.T) {
	svc, s := newTestService(t)
	destDir := t.TempDir()
	setCloudConfig(t, s, func(c *store.CloudConfig) {
		c.Enabled = true
		c.Provider = "local"
		c.URL = destDir
	})

	src := writeTempZip(t, "zip bytes")
	if err := svc.Upload(src, "game__main__snap_1.zip"); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	files, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Name != "game__main__snap_1.zip" {
		t.Fatalf("List = %+v", files)
	}

	dl := filepath.Join(t.TempDir(), "restored.zip")
	if err := svc.Download("game__main__snap_1.zip", dl); err != nil {
		t.Fatalf("Download: %v", err)
	}
	got, _ := os.ReadFile(dl)
	if string(got) != "zip bytes" {
		t.Errorf("downloaded = %q", got)
	}
}

func TestLocalFolderUploadCannotReplaceExistingSnapshot(t *testing.T) {
	svc, db := newTestService(t)
	dest := t.TempDir()
	setCloudConfig(t, db, func(c *store.CloudConfig) { c.Enabled, c.Provider, c.URL = true, "local", dest })
	name := "game__main__snap.zip"
	if err := svc.Upload(writeTempZip(t, "first"), name); err != nil {
		t.Fatal(err)
	}
	if err := svc.Upload(writeTempZip(t, "second"), name); !os.IsExist(err) {
		t.Fatalf("second upload = %v, want already-exists", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, name))
	if err != nil || string(got) != "first" {
		t.Fatalf("existing remote snapshot changed: %q, %v", got, err)
	}
}

func TestLocalFolderListingErrorIsNotEmptyRemote(t *testing.T) {
	svc, db := newTestService(t)
	notDirectory := writeTempZip(t, "not a directory")
	setCloudConfig(t, db, func(c *store.CloudConfig) { c.Enabled, c.Provider, c.URL = true, "local", notDirectory })
	if err := svc.UploadIfAbsent("source.zip", "game__main__snap.zip"); err == nil {
		t.Fatal("listing a non-directory was treated as an empty destination")
	}
}

func TestWebDAVConditionalUploadRejectsExistingSnapshot(t *testing.T) {
	var condition string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			condition = r.Header.Get("If-None-Match")
			w.WriteHeader(http.StatusPreconditionFailed)
		}
	}))
	defer server.Close()
	svc, db := newTestService(t)
	setCloudConfig(t, db, func(c *store.CloudConfig) {
		c.Enabled, c.Provider, c.URL = true, "webdav", server.URL+"/dav"
		c.HeadersJSON = `{"If-None-Match":"unsafe-override"}`
	})
	if err := svc.Upload(writeTempZip(t, "new"), "game__main__snap.zip"); !errors.Is(err, ErrRemoteSnapshotConflict) {
		t.Fatalf("WebDAV collision = %v", err)
	}
	if condition != "*" {
		t.Fatalf("If-None-Match = %q", condition)
	}
}

func TestWebDAVProvider(t *testing.T) {
	var uploaded []byte
	var gotAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		switch r.Method {
		case http.MethodPut:
			uploaded, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusCreated)
		case http.MethodHead:
			// Post-upload verification: report what was actually stored.
			w.Header().Set("Content-Length", fmt.Sprint(len(uploaded)))
			w.WriteHeader(http.StatusOK)
		case "PROPFIND":
			w.WriteHeader(http.StatusMultiStatus)
			fmt.Fprint(w, `<?xml version="1.0"?>
<D:multistatus xmlns:D="DAV:">
  <D:response>
    <D:href>/dav/</D:href>
    <D:propstat><D:prop><D:getcontentlength>0</D:getcontentlength></D:prop></D:propstat>
  </D:response>
  <D:response>
    <D:href>/dav/game__main__snap_9.zip</D:href>
    <D:propstat><D:prop>
      <D:getcontentlength>1234</D:getcontentlength>
      <D:getlastmodified>Wed, 01 Jul 2026 10:00:00 GMT</D:getlastmodified>
    </D:prop></D:propstat>
  </D:response>
</D:multistatus>`)
		case http.MethodGet:
			fmt.Fprint(w, "webdav content")
		}
	}))
	defer server.Close()

	svc, s := newTestService(t)
	setCloudConfig(t, s, func(c *store.CloudConfig) {
		c.Enabled = true
		c.Provider = "webdav"
		c.URL = server.URL + "/dav"
		c.Username = "user"
		c.Password = "pass"
	})

	if err := svc.Upload(writeTempZip(t, "dav data"), "game__main__snap_9.zip"); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if string(uploaded) != "dav data" {
		t.Errorf("server received %q", uploaded)
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass"))
	if gotAuth != wantAuth {
		t.Errorf("auth header = %q, want %q", gotAuth, wantAuth)
	}

	files, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 1 || files[0].Name != "game__main__snap_9.zip" || files[0].SizeBytes != 1234 {
		t.Errorf("List = %+v", files)
	}

	dl := filepath.Join(t.TempDir(), "out.zip")
	if err := svc.Download("game__main__snap_9.zip", dl); err != nil {
		t.Fatalf("Download: %v", err)
	}
	got, _ := os.ReadFile(dl)
	if string(got) != "webdav content" {
		t.Errorf("downloaded = %q", got)
	}
}

func TestGoogleDriveProviderWithTokenRefresh(t *testing.T) {
	refreshCalls := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Relay OAuth proxy: refresh grant.
		refreshCalls++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["grant_type"] != "refresh_token" || body["refresh_token"] != "rt-old" {
			t.Errorf("unexpected proxy payload: %v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at-fresh", "expires_in": 3600})
	}))
	defer proxy.Close()

	var driveURL string
	uploaded := &bytes.Buffer{}
	drive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Chunk PUTs go to the pre-authorized session URL (no auth header
		// required, mirroring the real API).
		if r.URL.Path == "/resumable-session" {
			_, _ = io.Copy(uploaded, r.Body)
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"id":"file123"}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer at-fresh" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case strings.HasPrefix(r.URL.Path, "/upload/"):
			// Resumable initiation: hand back the session URL.
			w.Header().Set("Location", driveURL+"/resumable-session")
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/drive/v3/files":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"files": []map[string]any{
					{"id": "f1", "name": "game__main__snap_5.zip", "size": "2048", "createdTime": "2026-07-01T00:00:00Z"},
				},
			})
		case strings.HasPrefix(r.URL.Path, "/drive/v3/files/f1"):
			fmt.Fprint(w, "drive bytes")
		}
	}))
	defer drive.Close()
	driveURL = drive.URL

	svc, s := newTestService(t)
	// Point the relay (and thus the OAuth proxy) at the mock.
	settings, _ := s.GetSettings()
	settings.RelayURL = strings.Replace(proxy.URL, "http://", "ws://", 1)
	if err := s.UpdateSettings(settings); err != nil {
		t.Fatal(err)
	}
	setCloudConfig(t, s, func(c *store.CloudConfig) {
		c.Enabled = true
		c.Provider = "google_drive"
		c.AccessToken = "at-expired"
		c.RefreshToken = "rt-old"
		c.ExpiryTimeMs = time.Now().UnixMilli() - 1000 // already expired -> must refresh
	})
	svc.Endpoints.GoogleAPI = drive.URL
	svc.Endpoints.GoogleUpload = drive.URL

	if err := svc.Upload(writeTempZip(t, "drive data"), "game__main__snap_5.zip"); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if refreshCalls != 1 {
		t.Errorf("refresh calls = %d, want 1", refreshCalls)
	}

	// The refreshed token must be persisted (no second refresh).
	files, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 1 || files[0].SizeBytes != 2048 {
		t.Errorf("List = %+v", files)
	}
	if refreshCalls != 1 {
		t.Errorf("token should be reused after refresh, refresh calls = %d", refreshCalls)
	}

	dl := filepath.Join(t.TempDir(), "out.zip")
	if err := svc.Download("game__main__snap_5.zip", dl); err != nil {
		t.Fatalf("Download: %v", err)
	}
	got, _ := os.ReadFile(dl)
	if string(got) != "drive bytes" {
		t.Errorf("downloaded = %q", got)
	}
}

func TestGoogleDriveListingFindsCollisionOnLaterPage(t *testing.T) {
	requests := 0
	uploads := 0
	drive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/upload/") {
			uploads++
			w.WriteHeader(http.StatusCreated)
			return
		}
		requests++
		if r.URL.Query().Get("pageSize") != "1000" || !strings.Contains(r.URL.Query().Get("fields"), "nextPageToken") {
			t.Errorf("pagination fields missing: %s", r.URL.RawQuery)
		}
		switch r.URL.Query().Get("pageToken") {
		case "":
			fmt.Fprint(w, `{"nextPageToken":"second","files":[{"id":"one","name":"other.zip","size":"1"}]}`)
		case "second":
			fmt.Fprint(w, `{"files":[{"id":"two","name":"game__main__snap.zip","size":"9"}]}`)
		default:
			t.Errorf("unexpected page token: %s", r.URL.Query().Get("pageToken"))
		}
	}))
	defer drive.Close()
	svc, db := newTestService(t)
	setCloudConfig(t, db, func(c *store.CloudConfig) {
		c.Enabled, c.Provider, c.FolderID = true, "google_drive", "folder"
		c.AccessToken, c.ExpiryTimeMs = "token", time.Now().Add(time.Hour).UnixMilli()
	})
	svc.Endpoints.GoogleAPI = drive.URL
	svc.Endpoints.GoogleUpload = drive.URL
	files, err := svc.List()
	if err != nil || len(files) != 2 || files[1].Name != "game__main__snap.zip" {
		t.Fatalf("paged listing = %#v, %v", files, err)
	}
	if err := svc.UploadIfAbsent("not-read", "game__main__snap.zip"); !errors.Is(err, ErrRemoteSnapshotConflict) {
		t.Fatalf("later-page collision = %v", err)
	}
	if requests != 4 || uploads != 0 {
		t.Fatalf("requests=%d uploads=%d", requests, uploads)
	}
}

func TestGoogleDriveIncompleteListingFailsClosed(t *testing.T) {
	for _, tc := range []struct{ name, payload string }{
		{"incomplete", `{"incompleteSearch":true,"files":[{"name":"first.zip"}]}`},
		{"repeated token", `{"nextPageToken":"same","files":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			drive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				fmt.Fprint(w, tc.payload)
			}))
			defer drive.Close()
			svc, db := newTestService(t)
			setCloudConfig(t, db, func(c *store.CloudConfig) {
				c.Enabled, c.Provider, c.FolderID = true, "google_drive", "folder"
				c.AccessToken, c.ExpiryTimeMs = "token", time.Now().Add(time.Hour).UnixMilli()
			})
			svc.Endpoints.GoogleAPI = drive.URL
			if err := svc.UploadIfAbsent("not-read", "game__main__snap.zip"); err == nil {
				t.Fatal("incomplete Drive listing allowed an upload")
			}
			if calls > 2 {
				t.Fatalf("listing did not stop on malformed pagination: %d calls", calls)
			}
		})
	}
}

func TestGoogleDriveDownloadRejectsDuplicateNameAcrossPages(t *testing.T) {
	const name = "game__main__snap.zip"
	mediaCalls := 0
	drive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/drive/v3/files" && r.URL.Query().Get("pageToken") == "":
			fmt.Fprintf(w, `{"nextPageToken":"second","files":[{"id":"first","name":%q}]}`, name)
		case r.URL.Path == "/drive/v3/files" && r.URL.Query().Get("pageToken") == "second":
			fmt.Fprintf(w, `{"files":[{"id":"second","name":%q}]}`, name)
		case strings.HasPrefix(r.URL.Path, "/drive/v3/files/"):
			mediaCalls++
			fmt.Fprint(w, "unexpected download")
		default:
			t.Errorf("unexpected Drive request: %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer drive.Close()
	svc, db := newTestService(t)
	setCloudConfig(t, db, func(c *store.CloudConfig) {
		c.Enabled, c.Provider, c.FolderID = true, "google_drive", "folder"
		c.AccessToken, c.ExpiryTimeMs = "token", time.Now().Add(time.Hour).UnixMilli()
	})
	svc.Endpoints.GoogleAPI = drive.URL
	dest := filepath.Join(t.TempDir(), "out.zip")
	if err := svc.Download(name, dest); !errors.Is(err, ErrRemoteSnapshotAmbiguous) {
		t.Fatalf("duplicate-name download error = %v", err)
	}
	if mediaCalls != 0 {
		t.Fatalf("media downloaded despite ambiguity: %d requests", mediaCalls)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("destination was created despite ambiguity: %v", err)
	}
}

func TestGoogleDriveDownloadFindsUniqueNameOnLaterPage(t *testing.T) {
	const name = "game__main__snap.zip"
	mediaCalls := 0
	drive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/drive/v3/files" && r.URL.Query().Get("pageToken") == "":
			fmt.Fprint(w, `{"nextPageToken":"second","files":[{"id":"first","name":"other.zip"}]}`)
		case r.URL.Path == "/drive/v3/files" && r.URL.Query().Get("pageToken") == "second":
			fmt.Fprintf(w, `{"files":[{"id":"target","name":%q}]}`, name)
		case r.URL.Path == "/drive/v3/files/target" && r.URL.Query().Get("alt") == "media":
			mediaCalls++
			fmt.Fprint(w, "expected bytes")
		default:
			t.Errorf("unexpected Drive request: %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer drive.Close()
	svc, db := newTestService(t)
	setCloudConfig(t, db, func(c *store.CloudConfig) {
		c.Enabled, c.Provider, c.FolderID = true, "google_drive", "folder"
		c.AccessToken, c.ExpiryTimeMs = "token", time.Now().Add(time.Hour).UnixMilli()
	})
	svc.Endpoints.GoogleAPI = drive.URL
	dest := filepath.Join(t.TempDir(), "out.zip")
	if err := svc.Download(name, dest); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "expected bytes" || mediaCalls != 1 {
		t.Fatalf("download bytes=%q, error=%v, media calls=%d", data, err, mediaCalls)
	}
}

func TestGoogleDriveDownloadRejectsIncompleteListing(t *testing.T) {
	mediaCalls := 0
	drive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/drive/v3/files" {
			fmt.Fprint(w, `{"incompleteSearch":true,"files":[{"id":"target","name":"game__main__snap.zip"}]}`)
			return
		}
		mediaCalls++
		fmt.Fprint(w, "unexpected download")
	}))
	defer drive.Close()
	svc, db := newTestService(t)
	setCloudConfig(t, db, func(c *store.CloudConfig) {
		c.Enabled, c.Provider, c.FolderID = true, "google_drive", "folder"
		c.AccessToken, c.ExpiryTimeMs = "token", time.Now().Add(time.Hour).UnixMilli()
	})
	svc.Endpoints.GoogleAPI = drive.URL
	dest := filepath.Join(t.TempDir(), "out.zip")
	if err := svc.Download("game__main__snap.zip", dest); err == nil {
		t.Fatal("incomplete listing allowed download")
	}
	if mediaCalls != 0 {
		t.Fatalf("media downloaded despite incomplete listing: %d requests", mediaCalls)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("destination was created despite incomplete listing: %v", err)
	}
}

func TestGoogleDriveDownloadRejectsMissingFileID(t *testing.T) {
	mediaCalls := 0
	drive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/drive/v3/files" {
			fmt.Fprint(w, `{"files":[{"name":"game__main__snap.zip"}]}`)
			return
		}
		mediaCalls++
	}))
	defer drive.Close()
	svc, db := newTestService(t)
	setCloudConfig(t, db, func(c *store.CloudConfig) {
		c.Enabled, c.Provider, c.FolderID = true, "google_drive", "folder"
		c.AccessToken, c.ExpiryTimeMs = "token", time.Now().Add(time.Hour).UnixMilli()
	})
	svc.Endpoints.GoogleAPI = drive.URL
	dest := filepath.Join(t.TempDir(), "out.zip")
	if err := svc.Download("game__main__snap.zip", dest); err == nil {
		t.Fatal("missing file ID allowed download")
	}
	if mediaCalls != 0 {
		t.Fatalf("media requested without file ID: %d", mediaCalls)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("destination was created without file ID: %v", err)
	}
}

// TestPruneGameBranch verifies cloud retention: newest `keep` snapshots
// stay, older ones are deleted, other games/branches are untouched.
func TestPruneGameBranch(t *testing.T) {
	svc, s := newTestService(t)
	destDir := t.TempDir()
	setCloudConfig(t, s, func(c *store.CloudConfig) {
		c.Enabled = true
		c.Provider = "local"
		c.URL = destDir
	})

	// Upload 5 snapshots for game__main plus one for another branch/game.
	names := []string{
		"game__main__snap_1.zip", "game__main__snap_2.zip", "game__main__snap_3.zip",
		"game__main__snap_4.zip", "game__main__snap_5.zip",
		"game__ngplus__snap_9.zip", "other__main__snap_1.zip",
	}
	for i, n := range names {
		if err := svc.Upload(writeTempZip(t, "data"), n); err != nil {
			t.Fatal(err)
		}
		// Stagger mtimes so CreatedTime ordering is deterministic.
		older := time.Now().Add(-time.Duration(len(names)-i) * time.Minute)
		_ = os.Chtimes(filepath.Join(destDir, n), older, older)
	}

	pruned, err := svc.PruneGameBranch(func(name string) bool {
		return strings.HasPrefix(name, "game__main__")
	}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if pruned != 2 {
		t.Errorf("pruned = %d, want 2", pruned)
	}

	remaining, _ := svc.List()
	got := map[string]bool{}
	for _, f := range remaining {
		got[f.Name] = true
	}
	for _, want := range []string{"game__main__snap_3.zip", "game__main__snap_4.zip", "game__main__snap_5.zip", "game__ngplus__snap_9.zip", "other__main__snap_1.zip"} {
		if !got[want] {
			t.Errorf("%s should have been kept", want)
		}
	}
	for _, gone := range []string{"game__main__snap_1.zip", "game__main__snap_2.zip"} {
		if got[gone] {
			t.Errorf("%s should have been pruned", gone)
		}
	}

	// keep <= 0 disables pruning entirely.
	if n, _ := svc.PruneGameBranch(func(string) bool { return true }, 0); n != 0 {
		t.Errorf("keep=0 pruned %d files; retention should be disabled", n)
	}
}

// TestRefreshInvalidGrantClearsTokens verifies that a permanently-dead
// refresh token (Google's invalid_grant) wipes the stored credentials so
// the UI stops showing "connected" and the user is told to reconnect.
func TestRefreshInvalidGrantClearsTokens(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`)
	}))
	defer proxy.Close()

	svc, s := newTestService(t)
	settings, _ := s.GetSettings()
	settings.RelayURL = strings.Replace(proxy.URL, "http://", "ws://", 1)
	if err := s.UpdateSettings(settings); err != nil {
		t.Fatal(err)
	}
	setCloudConfig(t, s, func(c *store.CloudConfig) {
		c.Enabled = true
		c.Provider = "google_drive"
		c.AccessToken = "at-expired"
		c.RefreshToken = "rt-dead"
		c.ExpiryTimeMs = time.Now().UnixMilli() - 1000 // forces a refresh
		c.UserEmail = "player@example.com"
	})

	err := svc.Upload(writeTempZip(t, "data"), "game__main__snap_1.zip")
	if err == nil {
		t.Fatal("expected upload to fail on dead refresh token")
	}
	if !strings.Contains(err.Error(), "expired") || !strings.Contains(err.Error(), "reconnect") {
		t.Errorf("error should tell the user to reconnect, got: %v", err)
	}

	// Dead credentials must be wiped so the UI shows "disconnected".
	cfg, _ := s.GetCloudConfig()
	if cfg.AccessToken != "" || cfg.RefreshToken != "" || cfg.UserEmail != "" || cfg.ExpiryTimeMs != 0 {
		t.Errorf("dead tokens should be cleared, got %+v", cfg)
	}
}

func TestDropboxProvider(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/2/files/list_folder" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"has_more": false,
				"entries": []map[string]any{
					{".tag": "file", "name": "game__main__snap_7.zip", "size": 512, "client_modified": "2026-07-01T00:00:00Z"},
					{".tag": "folder", "name": "subfolder"},
				},
			})
		}
	}))
	defer api.Close()

	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2/files/upload":
			var args map[string]any
			_ = json.Unmarshal([]byte(r.Header.Get("Dropbox-API-Arg")), &args)
			if args["path"] != "/OpenSave/game__main__snap_7.zip" {
				t.Errorf("upload path = %v", args["path"])
			}
			if args["mode"] != "add" || args["autorename"] != false || args["strict_conflict"] != true {
				t.Errorf("unsafe Dropbox upload args = %#v", args)
			}
			fmt.Fprint(w, `{}`)
		case "/2/files/download":
			fmt.Fprint(w, "dropbox bytes")
		}
	}))
	defer content.Close()

	svc, s := newTestService(t)
	setCloudConfig(t, s, func(c *store.CloudConfig) {
		c.Enabled = true
		c.Provider = "dropbox"
		c.AccessToken = "at-db"
		c.ExpiryTimeMs = time.Now().UnixMilli() + 3600_000
	})
	svc.Endpoints.DropboxAPI = api.URL
	svc.Endpoints.DropboxContent = content.URL

	if err := svc.Upload(writeTempZip(t, "db data"), "game__main__snap_7.zip"); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	files, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Name != "game__main__snap_7.zip" {
		t.Errorf("List = %+v (folders must be filtered)", files)
	}
	dl := filepath.Join(t.TempDir(), "out.zip")
	if err := svc.Download("game__main__snap_7.zip", dl); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dl)
	if string(got) != "dropbox bytes" {
		t.Errorf("downloaded = %q", got)
	}
}

func TestDropboxUploadRaceDoesNotOverwrite(t *testing.T) {
	for _, session := range []bool{false, true} {
		name := "simple"
		if session {
			name = "session"
		}
		t.Run(name, func(t *testing.T) {
			if session {
				old := dropboxSessionThreshold
				dropboxSessionThreshold = 1
				defer func() { dropboxSessionThreshold = old }()
			}
			finishCalls := 0
			content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/start") {
					fmt.Fprint(w, `{"session_id":"test-session"}`)
					return
				}
				var args map[string]any
				if err := json.Unmarshal([]byte(r.Header.Get("Dropbox-API-Arg")), &args); err != nil {
					t.Errorf("invalid Dropbox upload args: %v", err)
				}
				if strings.HasSuffix(r.URL.Path, "/finish") {
					finishCalls++
					args, _ = args["commit"].(map[string]any)
				}
				if args["mode"] != "add" || args["autorename"] != false || args["strict_conflict"] != true {
					t.Errorf("unsafe Dropbox commit: %#v", args)
				}
				w.WriteHeader(http.StatusConflict)
				fmt.Fprint(w, `{"error_summary":"path/conflict/file/"}`)
			}))
			defer content.Close()
			svc, db := newTestService(t)
			setCloudConfig(t, db, func(c *store.CloudConfig) {
				c.Enabled, c.Provider = true, "dropbox"
				c.AccessToken, c.ExpiryTimeMs = "at-db", time.Now().Add(time.Hour).UnixMilli()
			})
			svc.Endpoints.DropboxContent = content.URL
			if err := svc.Upload(writeTempZip(t, "bytes"), "game__main__snap.zip"); !errors.Is(err, ErrRemoteSnapshotConflict) {
				t.Fatalf("Dropbox upload race = %v", err)
			}
			if session && finishCalls != 1 {
				t.Fatalf("session finish calls = %d", finishCalls)
			}
		})
	}
}

func TestDropboxConcurrentUploadIfAbsentKeepsFirstWriter(t *testing.T) {
	var mu sync.Mutex
	lists, writes := 0, 0
	var saved string
	listed := make(chan struct{})
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lists++
		if lists == 2 {
			close(listed)
		}
		mu.Unlock()
		select {
		case <-listed:
		case <-time.After(5 * time.Second):
			t.Error("second Dropbox listing never arrived")
			w.WriteHeader(http.StatusGatewayTimeout)
			return
		}
		fmt.Fprint(w, `{"entries":[],"has_more":false}`)
	}))
	defer api.Close()
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		if writes > 0 {
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"error_summary":"path/conflict/file/"}`)
			return
		}
		writes++
		saved = string(body)
		fmt.Fprint(w, `{}`)
	}))
	defer content.Close()
	results := make(chan error, 2)
	for _, payload := range []string{"device-a", "device-b"} {
		svc, db := newTestService(t)
		setCloudConfig(t, db, func(c *store.CloudConfig) {
			c.Enabled, c.Provider = true, "dropbox"
			c.AccessToken, c.ExpiryTimeMs = "at-db", time.Now().Add(time.Hour).UnixMilli()
		})
		svc.Endpoints.DropboxAPI, svc.Endpoints.DropboxContent = api.URL, content.URL
		path := writeTempZip(t, payload)
		go func() { results <- svc.UploadIfAbsent(path, "game__main__same.zip") }()
	}
	first, second := <-results, <-results
	if (first == nil) == (second == nil) ||
		(first != nil && !errors.Is(first, ErrRemoteSnapshotConflict)) ||
		(second != nil && !errors.Is(second, ErrRemoteSnapshotConflict)) {
		t.Fatalf("concurrent upload results = %v, %v", first, second)
	}
	mu.Lock()
	defer mu.Unlock()
	if lists != 2 || writes != 1 || (saved != "device-a" && saved != "device-b") {
		t.Fatalf("lists=%d writes=%d saved=%q", lists, writes, saved)
	}
}

func TestDropboxListingFindsCollisionOnLaterPage(t *testing.T) {
	requests, uploads := 0, 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") != "Bearer at-db" {
			t.Errorf("missing Dropbox authorization")
		}
		switch r.URL.Path {
		case "/2/files/list_folder":
			fmt.Fprint(w, `{"entries":[{".tag":"file","name":"other.zip"}],"cursor":"second","has_more":true}`)
		case "/2/files/list_folder/continue":
			var args struct {
				Cursor string `json:"cursor"`
			}
			if err := json.NewDecoder(r.Body).Decode(&args); err != nil || args.Cursor != "second" {
				t.Errorf("continuation cursor = %q, %v", args.Cursor, err)
			}
			fmt.Fprint(w, `{"entries":[{".tag":"file","name":"game__main__snap.zip"}],"cursor":"end","has_more":false}`)
		default:
			t.Errorf("unexpected Dropbox request: %s", r.URL.Path)
		}
	}))
	defer api.Close()
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { uploads++ }))
	defer content.Close()
	svc, db := newTestService(t)
	setCloudConfig(t, db, func(c *store.CloudConfig) {
		c.Enabled, c.Provider = true, "dropbox"
		c.AccessToken, c.ExpiryTimeMs = "at-db", time.Now().Add(time.Hour).UnixMilli()
	})
	svc.Endpoints.DropboxAPI, svc.Endpoints.DropboxContent = api.URL, content.URL
	if err := svc.UploadIfAbsent("not-read", "game__main__snap.zip"); !errors.Is(err, ErrRemoteSnapshotConflict) {
		t.Fatalf("later-page collision = %v", err)
	}
	if requests != 2 || uploads != 0 {
		t.Fatalf("requests=%d uploads=%d", requests, uploads)
	}
}

func TestDropboxIncompleteListingFailsClosed(t *testing.T) {
	for _, tc := range []struct{ name, first, next string }{
		{"missing has_more", `{"entries":[]}`, ""},
		{"missing entries", `{"has_more":false}`, ""},
		{"missing cursor", `{"entries":[],"has_more":true}`, ""},
		{"repeated cursor", `{"entries":[],"cursor":"same","has_more":true}`, `{"entries":[],"cursor":"same","has_more":true}`},
		{"unknown conflict", `{"error_summary":"path/not_folder/"}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if tc.name == "unknown conflict" {
					w.WriteHeader(http.StatusConflict)
				}
				if r.URL.Path == "/2/files/list_folder/continue" {
					fmt.Fprint(w, tc.next)
				} else {
					fmt.Fprint(w, tc.first)
				}
			}))
			defer api.Close()
			svc, db := newTestService(t)
			setCloudConfig(t, db, func(c *store.CloudConfig) {
				c.Enabled, c.Provider = true, "dropbox"
				c.AccessToken, c.ExpiryTimeMs = "at-db", time.Now().Add(time.Hour).UnixMilli()
			})
			svc.Endpoints.DropboxAPI = api.URL
			if err := svc.UploadIfAbsent("not-read", "game__main__snap.zip"); err == nil {
				t.Fatal("incomplete Dropbox listing allowed an upload")
			}
			if calls > 2 {
				t.Fatalf("listing did not stop: %d calls", calls)
			}
		})
	}
}

func TestDropboxMissingFolderIsEmpty(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, `{"error_summary":"path/not_found/..."}`)
	}))
	defer api.Close()
	svc, db := newTestService(t)
	setCloudConfig(t, db, func(c *store.CloudConfig) {
		c.Enabled, c.Provider = true, "dropbox"
		c.AccessToken, c.ExpiryTimeMs = "at-db", time.Now().Add(time.Hour).UnixMilli()
	})
	svc.Endpoints.DropboxAPI = api.URL
	files, err := svc.List()
	if err != nil || len(files) != 0 {
		t.Fatalf("missing folder listing = %#v, %v", files, err)
	}
}

func TestDropboxLaterPageFailureStopsUpload(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/2/files/list_folder/continue" {
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"error_summary":"too_many_requests/"}`)
			return
		}
		fmt.Fprint(w, `{"entries":[{".tag":"file","name":"other.zip"}],"cursor":"second","has_more":true}`)
	}))
	defer api.Close()
	svc, db := newTestService(t)
	setCloudConfig(t, db, func(c *store.CloudConfig) {
		c.Enabled, c.Provider = true, "dropbox"
		c.AccessToken, c.ExpiryTimeMs = "at-db", time.Now().Add(time.Hour).UnixMilli()
	})
	svc.Endpoints.DropboxAPI = api.URL
	if err := svc.UploadIfAbsent("not-read", "game__main__snap.zip"); err == nil {
		t.Fatal("later-page Dropbox error allowed an upload")
	}
}

func TestOneDriveProvider(t *testing.T) {
	graph := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/children"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"value": []map[string]any{
					{"name": "game__main__snap_3.zip", "size": 999, "createdDateTime": "2026-07-01T00:00:00Z", "file": map[string]any{}},
					{"name": "notazip.txt", "size": 1, "file": map[string]any{}},
				},
			})
		case r.Method == http.MethodPut:
			if r.URL.Query().Get("@microsoft.graph.conflictBehavior") != "fail" {
				t.Errorf("OneDrive small upload may replace an existing file: %s", r.URL.RawQuery)
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{}`)
		case r.Method == http.MethodGet:
			fmt.Fprint(w, "onedrive bytes")
		}
	}))
	defer graph.Close()

	svc, s := newTestService(t)
	setCloudConfig(t, s, func(c *store.CloudConfig) {
		c.Enabled = true
		c.Provider = "onedrive"
		c.AccessToken = "at-od"
		c.ExpiryTimeMs = time.Now().UnixMilli() + 3600_000
	})
	svc.Endpoints.Graph = graph.URL

	if err := svc.Upload(writeTempZip(t, "od data"), "game__main__snap_3.zip"); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	files, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Name != "game__main__snap_3.zip" {
		t.Errorf("List = %+v (non-zips must be filtered)", files)
	}
	dl := filepath.Join(t.TempDir(), "out.zip")
	if err := svc.Download("game__main__snap_3.zip", dl); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dl)
	if string(got) != "onedrive bytes" {
		t.Errorf("downloaded = %q", got)
	}
}

func TestOneDriveUploadRaceDoesNotOverwrite(t *testing.T) {
	for _, session := range []bool{false, true} {
		name := "simple"
		if session {
			name = "session"
		}
		t.Run(name, func(t *testing.T) {
			if session {
				old := onedriveSimpleLimit
				onedriveSimpleLimit = 1
				defer func() { onedriveSimpleLimit = old }()
			}
			var graph *httptest.Server
			graph = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/createUploadSession") {
					var args struct {
						Item map[string]string `json:"item"`
					}
					if err := json.NewDecoder(r.Body).Decode(&args); err != nil || args.Item["@microsoft.graph.conflictBehavior"] != "fail" {
						t.Errorf("unsafe OneDrive session args: %#v, %v", args, err)
					}
					fmt.Fprintf(w, `{"uploadUrl":%q}`, graph.URL+"/session-upload")
					return
				}
				if r.URL.Path != "/session-upload" && r.URL.Query().Get("@microsoft.graph.conflictBehavior") != "fail" {
					t.Errorf("unsafe OneDrive small upload: %s", r.URL.RawQuery)
				}
				w.WriteHeader(http.StatusConflict)
				fmt.Fprint(w, `{"error":{"code":"nameAlreadyExists"}}`)
			}))
			defer graph.Close()
			svc, db := newTestService(t)
			setCloudConfig(t, db, func(c *store.CloudConfig) {
				c.Enabled, c.Provider = true, "onedrive"
				c.AccessToken, c.ExpiryTimeMs = "at-od", time.Now().Add(time.Hour).UnixMilli()
			})
			svc.Endpoints.Graph = graph.URL
			if err := svc.Upload(writeTempZip(t, "bytes"), "game__main__snap.zip"); !errors.Is(err, ErrRemoteSnapshotConflict) {
				t.Fatalf("OneDrive upload race = %v", err)
			}
		})
	}
}

func TestOneDriveConcurrentUploadIfAbsentKeepsFirstWriter(t *testing.T) {
	var mu sync.Mutex
	lists, writes := 0, 0
	var saved string
	listed := make(chan struct{})
	graph := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/children") {
			mu.Lock()
			lists++
			if lists == 2 {
				close(listed)
			}
			mu.Unlock()
			select {
			case <-listed:
			case <-time.After(5 * time.Second):
				t.Error("second OneDrive listing never arrived")
				w.WriteHeader(http.StatusGatewayTimeout)
				return
			}
			fmt.Fprint(w, `{"value":[]}`)
			return
		}
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		if writes > 0 {
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"error":{"code":"nameAlreadyExists"}}`)
			return
		}
		writes++
		saved = string(body)
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{}`)
	}))
	defer graph.Close()
	results := make(chan error, 2)
	for _, payload := range []string{"device-a", "device-b"} {
		svc, db := newTestService(t)
		setCloudConfig(t, db, func(c *store.CloudConfig) {
			c.Enabled, c.Provider = true, "onedrive"
			c.AccessToken, c.ExpiryTimeMs = "at-od", time.Now().Add(time.Hour).UnixMilli()
		})
		svc.Endpoints.Graph = graph.URL
		path := writeTempZip(t, payload)
		go func() { results <- svc.UploadIfAbsent(path, "game__main__same.zip") }()
	}
	first, second := <-results, <-results
	if (first == nil) == (second == nil) ||
		(first != nil && !errors.Is(first, ErrRemoteSnapshotConflict)) ||
		(second != nil && !errors.Is(second, ErrRemoteSnapshotConflict)) {
		t.Fatalf("concurrent upload results = %v, %v", first, second)
	}
	mu.Lock()
	defer mu.Unlock()
	if lists != 2 || writes != 1 || (saved != "device-a" && saved != "device-b") {
		t.Fatalf("lists=%d writes=%d saved=%q", lists, writes, saved)
	}
}

func TestOneDriveListingFindsCollisionOnLaterPage(t *testing.T) {
	requests, uploads := 0, 0
	var graph *httptest.Server
	graph = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") != "Bearer at-od" {
			t.Errorf("missing OneDrive authorization")
		}
		if r.Method != http.MethodGet {
			uploads++
			return
		}
		if r.URL.Query().Get("page") == "2" {
			fmt.Fprint(w, `{"value":[{"name":"game__main__snap.zip","file":{},"size":9}]}`)
		} else {
			fmt.Fprintf(w, `{"@odata.nextLink":%q,"value":[{"name":"other.zip","file":{}}]}`,
				graph.URL+r.URL.Path+"?page=2")
		}
	}))
	defer graph.Close()
	svc, db := newTestService(t)
	setCloudConfig(t, db, func(c *store.CloudConfig) {
		c.Enabled, c.Provider = true, "onedrive"
		c.AccessToken, c.ExpiryTimeMs = "at-od", time.Now().Add(time.Hour).UnixMilli()
	})
	svc.Endpoints.Graph = graph.URL
	if err := svc.UploadIfAbsent("not-read", "game__main__snap.zip"); !errors.Is(err, ErrRemoteSnapshotConflict) {
		t.Fatalf("later-page collision = %v", err)
	}
	if requests != 2 || uploads != 0 {
		t.Fatalf("requests=%d uploads=%d", requests, uploads)
	}
}

func TestOneDriveUnsafeOrRepeatedNextLinkFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name     string
		nextLink func(string) string
	}{
		{"external origin", func(string) string { return "https://example.invalid/steal" }},
		{"other resource", func(base string) string { return base + "/v1.0/me/drive/root/children" }},
		{"repeated page", func(base string) string { return base + "/v1.0/me/drive/special/approot/children" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			var graph *httptest.Server
			graph = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				fmt.Fprintf(w, `{"@odata.nextLink":%q,"value":[]}`, tc.nextLink(graph.URL))
			}))
			defer graph.Close()
			svc, db := newTestService(t)
			setCloudConfig(t, db, func(c *store.CloudConfig) {
				c.Enabled, c.Provider = true, "onedrive"
				c.AccessToken, c.ExpiryTimeMs = "at-od", time.Now().Add(time.Hour).UnixMilli()
			})
			svc.Endpoints.Graph = graph.URL
			if err := svc.UploadIfAbsent("not-read", "game__main__snap.zip"); err == nil {
				t.Fatal("unsafe OneDrive listing allowed an upload")
			}
			if calls != 1 {
				t.Fatalf("unexpected follow-up request count = %d", calls)
			}
		})
	}
}

func TestOneDriveLaterPageFailureStopsUpload(t *testing.T) {
	var graph *httptest.Server
	graph = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"error":{"code":"throttled"}}`)
			return
		}
		fmt.Fprintf(w, `{"@odata.nextLink":%q,"value":[{"name":"other.zip","file":{}}]}`,
			graph.URL+r.URL.Path+"?page=2")
	}))
	defer graph.Close()
	svc, db := newTestService(t)
	setCloudConfig(t, db, func(c *store.CloudConfig) {
		c.Enabled, c.Provider = true, "onedrive"
		c.AccessToken, c.ExpiryTimeMs = "at-od", time.Now().Add(time.Hour).UnixMilli()
	})
	svc.Endpoints.Graph = graph.URL
	if err := svc.UploadIfAbsent("not-read", "game__main__snap.zip"); err == nil {
		t.Fatal("later-page OneDrive error allowed an upload")
	}
}

func TestOneDriveMissingValueStopsUpload(t *testing.T) {
	graph := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{}`)
	}))
	defer graph.Close()
	svc, db := newTestService(t)
	setCloudConfig(t, db, func(c *store.CloudConfig) {
		c.Enabled, c.Provider = true, "onedrive"
		c.AccessToken, c.ExpiryTimeMs = "at-od", time.Now().Add(time.Hour).UnixMilli()
	})
	svc.Endpoints.Graph = graph.URL
	if err := svc.UploadIfAbsent("not-read", "game__main__snap.zip"); err == nil {
		t.Fatal("missing OneDrive values allowed an upload")
	}
}

func TestWebhookUpload(t *testing.T) {
	var gotFileName string
	var gotHeader string
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Custom")
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Errorf("expected multipart form: %v", err)
			return
		}
		_, header, err := r.FormFile("file")
		if err == nil {
			gotFileName = header.Filename
		}
	}))
	defer hook.Close()

	svc, s := newTestService(t)
	setCloudConfig(t, s, func(c *store.CloudConfig) {
		c.Enabled = true
		c.Provider = "webhook"
		c.URL = hook.URL
		c.HeadersJSON = `{"X-Custom":"secret-token"}`
	})

	if err := svc.Upload(writeTempZip(t, "hook data"), "game__main__snap_2.zip"); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if gotFileName != "game__main__snap_2.zip" {
		t.Errorf("multipart filename = %q", gotFileName)
	}
	if gotHeader != "secret-token" {
		t.Errorf("custom header = %q", gotHeader)
	}
}

func TestPKCEAndAuthURL(t *testing.T) {
	svc, _ := newTestService(t)

	verifier, challenge := GeneratePKCE()
	if verifier == "" || challenge == "" || verifier == challenge {
		t.Fatal("bad PKCE pair")
	}
	if strings.ContainsAny(verifier+challenge, "+/=") {
		t.Error("PKCE values must be base64url without padding")
	}

	u, err := svc.AuthURL("dropbox", challenge)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(u, "dropbox.com/oauth2/authorize") ||
		!strings.Contains(u, "code_challenge="+challenge) ||
		!strings.Contains(u, "code_challenge_method=S256") {
		t.Errorf("dropbox auth URL wrong: %s", u)
	}

	if _, err := svc.AuthURL("onedrive", challenge); err == nil {
		t.Error("onedrive without a custom client ID must error (no built-in registration)")
	}

	u, err = svc.AuthURL("google_drive", challenge)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(u, "accounts.google.com") || !strings.Contains(u, "access_type=offline") {
		t.Errorf("google auth URL wrong: %s", u)
	}
}

func TestExchangeAuthCodePersistsTokens(t *testing.T) {
	token := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code") != "the-code" {
			t.Errorf("unexpected token form: %v", r.Form)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at-new", "refresh_token": "rt-new", "expires_in": 3600,
		})
	}))
	defer token.Close()

	profile := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"email": "user@example.com"})
	}))
	defer profile.Close()

	svc, s := newTestService(t)
	setCloudConfig(t, s, func(c *store.CloudConfig) {
		c.Provider = "dropbox"
	})
	svc.Endpoints.DropboxToken = token.URL
	svc.Endpoints.DropboxAPI = profile.URL

	if err := svc.ExchangeAuthCode("dropbox", "the-code", "the-verifier"); err != nil {
		t.Fatalf("ExchangeAuthCode: %v", err)
	}

	cfg, err := s.GetCloudConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AccessToken != "at-new" || cfg.RefreshToken != "rt-new" || cfg.UserEmail != "user@example.com" {
		t.Errorf("persisted config wrong: %+v", cfg)
	}
	if cfg.ExpiryTimeMs <= time.Now().UnixMilli() {
		t.Error("expiry must be in the future")
	}

	// Disconnect wipes tokens.
	if err := svc.Disconnect(); err != nil {
		t.Fatal(err)
	}
	cfg, _ = s.GetCloudConfig()
	if cfg.AccessToken != "" || cfg.RefreshToken != "" || cfg.UserEmail != "" {
		t.Errorf("disconnect must wipe tokens: %+v", cfg)
	}
}

// TestChunkedUploads shrinks the thresholds so a small file exercises the
// multi-chunk session protocols end to end.
func TestChunkedUploads(t *testing.T) {
	// 100 KB of data, 32 KB chunks -> 4 chunks.
	payload := bytes.Repeat([]byte("chunky-data-0123"), 6400)
	src := filepath.Join(t.TempDir(), "big.zip")
	if err := os.WriteFile(src, payload, 0o666); err != nil {
		t.Fatal(err)
	}

	t.Run("google drive resumable multi-chunk", func(t *testing.T) {
		old := driveChunkSize
		driveChunkSize = 32 << 10
		defer func() { driveChunkSize = old }()

		var driveURL string
		var got bytes.Buffer
		var chunkRanges []string
		drive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/session" {
				chunkRanges = append(chunkRanges, r.Header.Get("Content-Range"))
				_, _ = io.Copy(&got, r.Body)
				if strings.HasSuffix(r.Header.Get("Content-Range"), fmt.Sprintf("/%d", len(payload))) &&
					len(got.Bytes()) == len(payload) {
					w.WriteHeader(http.StatusOK)
				} else {
					w.WriteHeader(308)
				}
				return
			}
			w.Header().Set("Location", driveURL+"/session")
			w.WriteHeader(http.StatusOK)
		}))
		defer drive.Close()
		driveURL = drive.URL

		svc, s := newTestService(t)
		setCloudConfig(t, s, func(c *store.CloudConfig) {
			c.Enabled = true
			c.Provider = "google_drive"
			c.AccessToken = "at"
			c.ExpiryTimeMs = time.Now().UnixMilli() + 3600_000
			c.FolderID = "folder1"
		})
		svc.Endpoints.GoogleAPI = drive.URL
		svc.Endpoints.GoogleUpload = drive.URL

		if err := svc.Upload(src, "big__main__snap_1.zip"); err != nil {
			t.Fatalf("Upload: %v", err)
		}
		if !bytes.Equal(got.Bytes(), payload) {
			t.Errorf("reassembled %d bytes, want %d", got.Len(), len(payload))
		}
		if len(chunkRanges) != 4 {
			t.Errorf("chunks = %d (%v), want 4", len(chunkRanges), chunkRanges)
		}
	})

	t.Run("dropbox session multi-chunk", func(t *testing.T) {
		oldT, oldC := dropboxSessionThreshold, dropboxChunkSize
		dropboxSessionThreshold, dropboxChunkSize = 16<<10, 32<<10
		defer func() { dropboxSessionThreshold, dropboxChunkSize = oldT, oldC }()

		var got bytes.Buffer
		var calls []string
		content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls = append(calls, r.URL.Path)
			_, _ = io.Copy(&got, r.Body)
			if strings.HasSuffix(r.URL.Path, "/start") {
				_ = json.NewEncoder(w).Encode(map[string]any{"session_id": "sess1"})
				return
			}
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{}`)
		}))
		defer content.Close()

		svc, s := newTestService(t)
		setCloudConfig(t, s, func(c *store.CloudConfig) {
			c.Enabled = true
			c.Provider = "dropbox"
			c.AccessToken = "at"
			c.ExpiryTimeMs = time.Now().UnixMilli() + 3600_000
		})
		svc.Endpoints.DropboxContent = content.URL

		if err := svc.Upload(src, "big__main__snap_2.zip"); err != nil {
			t.Fatalf("Upload: %v", err)
		}
		if !bytes.Equal(got.Bytes(), payload) {
			t.Errorf("reassembled %d bytes, want %d", got.Len(), len(payload))
		}
		if len(calls) < 4 || !strings.HasSuffix(calls[0], "/start") || !strings.HasSuffix(calls[len(calls)-1], "/finish") {
			t.Errorf("session call sequence = %v", calls)
		}
	})

	t.Run("onedrive session multi-chunk", func(t *testing.T) {
		oldL, oldC := onedriveSimpleLimit, onedriveChunkSize
		onedriveSimpleLimit, onedriveChunkSize = 16<<10, 32<<10
		defer func() { onedriveSimpleLimit, onedriveChunkSize = oldL, oldC }()

		var graphURL string
		var got bytes.Buffer
		chunks := 0
		graph := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/session-upload" {
				chunks++
				_, _ = io.Copy(&got, r.Body)
				if got.Len() == len(payload) {
					w.WriteHeader(http.StatusCreated)
				} else {
					w.WriteHeader(http.StatusAccepted)
				}
				fmt.Fprint(w, `{}`)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"uploadUrl": graphURL + "/session-upload"})
		}))
		defer graph.Close()
		graphURL = graph.URL

		svc, s := newTestService(t)
		setCloudConfig(t, s, func(c *store.CloudConfig) {
			c.Enabled = true
			c.Provider = "onedrive"
			c.AccessToken = "at"
			c.ExpiryTimeMs = time.Now().UnixMilli() + 3600_000
		})
		svc.Endpoints.Graph = graph.URL

		if err := svc.Upload(src, "big__main__snap_3.zip"); err != nil {
			t.Fatalf("Upload: %v", err)
		}
		if !bytes.Equal(got.Bytes(), payload) {
			t.Errorf("reassembled %d bytes, want %d", got.Len(), len(payload))
		}
		if chunks != 4 {
			t.Errorf("chunks = %d, want 4", chunks)
		}
	})
}

// TestWebDAVUploadVerificationCatchesTruncation: a server that accepts
// the PUT but stores a truncated/empty file must fail the upload loudly
// (field report: an uploaded snapshot zip arrived empty and nobody knew).
func TestWebDAVUploadVerificationCatchesTruncation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusCreated) // "fine!" — but stores nothing
		case http.MethodHead:
			w.Header().Set("Content-Length", "0")
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	svc, s := newTestService(t)
	setCloudConfig(t, s, func(c *store.CloudConfig) {
		c.Enabled = true
		c.Provider = "webdav"
		c.URL = server.URL + "/dav"
	})

	err := svc.Upload(writeTempZip(t, "real data that must arrive"), "game__main__snap_1.zip")
	if err == nil {
		t.Fatal("truncated upload was reported as success")
	}
	if !strings.Contains(err.Error(), "verification failed") {
		t.Errorf("unexpected error: %v", err)
	}
}
