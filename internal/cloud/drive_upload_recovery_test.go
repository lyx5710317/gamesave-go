package cloud

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/store"
)

func TestGoogleDriveUploadRecoveryPreservesObjectIdentity(t *testing.T) {
	const (
		name    = "game__main__snap.zip"
		folder  = "fixture_folder"
		id      = "reserved_object"
		payload = "test archive"
	)
	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(payload)))
	for _, tc := range []struct {
		name              string
		firstResponse     int
		objectVisible     bool
		expired           bool
		checksum          string
		wantCreates       int
		wantChunks        int
		wantErr           bool
		finalID           string
		alwaysUnavailable bool
	}{
		{name: "lost initialization", firstResponse: 503, wantCreates: 2, wantChunks: 1},
		{name: "expired session", expired: true, wantCreates: 2, wantChunks: 2},
		{name: "expired session already committed", expired: true, objectVisible: true, checksum: checksum, wantCreates: 1, wantChunks: 1},
		{name: "409 confirms committed bytes", firstResponse: 409, objectVisible: true, checksum: checksum, wantCreates: 1},
		{name: "409 without confirmed object", firstResponse: 409, wantCreates: 1, wantErr: true},
		{name: "same size different content", firstResponse: 409, objectVisible: true, checksum: strings.Repeat("0", 64), wantCreates: 1, wantErr: true},
		{name: "missing checksum", firstResponse: 409, objectVisible: true, wantCreates: 1, wantErr: true},
		{name: "session creation retry is bounded", firstResponse: 503, alwaysUnavailable: true, wantCreates: 2, wantErr: true},
		{name: "server completed a different file ID", finalID: "other_id", wantCreates: 1, wantChunks: 1, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var driveURL string
			var reservations, creates, chunks, verification, listings int
			drive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/drive/v3/files/generateIds":
					reservations++
					if r.Method != http.MethodGet || r.URL.Query().Get("count") != "1" || r.Header.Get("Authorization") != "Bearer fixture-token" {
						t.Error("unexpected Drive ID reservation request")
					}
					fmt.Fprintf(w, `{"ids":[%q]}`, id)
				case "/upload/drive/v3/files":
					creates++
					var meta struct{ ID, Name string }
					if err := json.NewDecoder(r.Body).Decode(&meta); err != nil || meta.ID != id || meta.Name != name || r.Method != http.MethodPost {
						t.Error("a create did not reuse the reserved file ID and name")
					}
					if tc.firstResponse != 0 && (creates == 1 || tc.alwaysUnavailable) {
						w.WriteHeader(tc.firstResponse)
						return
					}
					w.Header().Set("Location", driveURL+"/fixture-session")
				case "/fixture-session":
					chunks++
					if tc.expired && chunks == 1 {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					completedID := id
					if tc.finalID != "" {
						completedID = tc.finalID
					}
					fmt.Fprintf(w, `{"id":%q,"name":%q,"size":"%d","parents":[%q]}`, completedID, name, len(payload), folder)
				case "/drive/v3/files/" + id:
					verification++
					if !tc.objectVisible {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					fmt.Fprintf(w, `{"id":%q,"name":%q,"size":"%d","parents":[%q],"mimeType":"application/zip","trashed":false,"sha256Checksum":%q}`, id, name, len(payload), folder, tc.checksum)
				case "/drive/v3/files":
					listings++
					fmt.Fprintf(w, `{"files":[{"id":%q,"name":%q,"size":"%d"}]}`, id, name, len(payload))
				default:
					t.Error("unexpected request in Drive recovery")
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(drive.Close)
			driveURL = drive.URL
			svc, db := newTestService(t)
			setCloudConfig(t, db, func(c *store.CloudConfig) {
				c.Enabled, c.Provider, c.FolderID = true, "google_drive", folder
				c.AccessToken, c.ExpiryTimeMs = "fixture-token", time.Now().Add(time.Hour).UnixMilli()
			})
			svc.Endpoints.GoogleAPI, svc.Endpoints.GoogleUpload = drive.URL, drive.URL
			err := svc.Upload(writeTempZip(t, payload), name)
			drive.Close()
			if tc.wantErr != (err != nil) || (tc.wantErr && !errors.Is(err, errUploadUnconfirmed)) {
				t.Fatalf("unexpected recovery result: %v", err)
			}
			wantVerification, wantListings := 1, 1
			if tc.wantErr {
				wantListings = 0
			}
			if tc.finalID != "" {
				wantVerification = 0
			} else if tc.alwaysUnavailable {
				wantVerification = 2
			}
			if reservations != 1 || creates != tc.wantCreates || chunks != tc.wantChunks || verification != wantVerification || listings != wantListings {
				t.Fatalf("reserved=%d creates=%d chunks=%d checked=%d listed=%d", reservations, creates, chunks, verification, listings)
			}
		})
	}
}
