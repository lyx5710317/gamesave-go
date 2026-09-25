package cloud

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/opensave/opensave/internal/store"
)

type jianguoyunDAVFixture struct {
	sync.Mutex
	folder          bool
	objects         map[string][]byte
	ignoreCondition bool
	wrongHead       bool
	interruptPUT    bool
	interruptGET    bool
	probeDeleteFail bool
	listXML         string
	listNextHeader  string
	realPuts        int
	probePuts       int
}

func (f *jianguoyunDAVFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.Lock()
	defer f.Unlock()
	const folder = "/dav/GameSaveGo/"
	if r.URL.Path == folder {
		switch r.Method {
		case "PROPFIND":
			if !f.folder {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if r.Header.Get("Depth") == "1" && f.listNextHeader != "" {
				w.Header().Set("Link", f.listNextHeader)
			}
			w.WriteHeader(http.StatusMultiStatus)
			if r.Header.Get("Depth") == "1" {
				if f.listXML != "" {
					fmt.Fprint(w, f.listXML)
					return
				}
				fmt.Fprint(w, `<D:multistatus xmlns:D="DAV:"><D:response><D:href>/dav/GameSaveGo/</D:href></D:response>`)
				for name, body := range f.objects {
					fmt.Fprintf(w, `<D:response><D:href>%s%s</D:href><D:propstat><D:prop><D:getcontentlength>%d</D:getcontentlength></D:prop></D:propstat></D:response>`, folder, url.PathEscape(name), len(body))
				}
				fmt.Fprint(w, `</D:multistatus>`)
			}
		case "MKCOL":
			f.folder = true
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
		return
	}
	if !strings.HasPrefix(r.URL.Path, folder) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, folder)
	if name == "" || strings.Contains(name, "/") {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if f.objects == nil {
		f.objects = make(map[string][]byte)
	}
	old, exists := f.objects[name]
	switch r.Method {
	case http.MethodPut:
		if strings.HasPrefix(name, ".gamesavego-condition-probe-") {
			f.probePuts++
		}
		if strings.HasSuffix(name, ".zip") && !strings.HasPrefix(name, ".gamesavego-condition-probe-") {
			f.realPuts++
			if f.interruptPUT {
				if hijacker, ok := w.(http.Hijacker); ok {
					conn, _, _ := hijacker.Hijack()
					conn.Close()
					return
				}
			}
		}
		if exists && !f.ignoreCondition && r.Header.Get("If-None-Match") == "*" {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		body, _ := io.ReadAll(r.Body)
		f.objects[name] = body
		w.WriteHeader(http.StatusCreated)
	case http.MethodGet:
		if !exists {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if f.interruptGET {
			if hijacker, ok := w.(http.Hijacker); ok {
				conn, _, _ := hijacker.Hijack()
				fmt.Fprint(conn, "HTTP/1.1 200 OK\r\nContent-Length: 1000\r\n\r\npartial")
				conn.Close()
				return
			}
		}
		w.Write(old)
	case http.MethodHead:
		if !exists {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		size := len(old)
		if f.wrongHead && strings.HasSuffix(name, ".zip") {
			size = 0
		}
		w.Header().Set("Content-Length", fmt.Sprint(size))
		w.WriteHeader(http.StatusOK)
	case http.MethodDelete:
		if f.probeDeleteFail && strings.HasPrefix(name, ".gamesavego-condition-probe-") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		delete(f.objects, name)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func newJianguoyunService(t *testing.T, fixture *jianguoyunDAVFixture) *Service {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("Jianguoyun password storage requires Windows Credential Manager")
	}
	oldInterval, oldRetry := jianguoyunRequestInterval, jianguoyunRetryBase
	jianguoyunRequestInterval, jianguoyunRetryBase = 0, 0
	t.Cleanup(func() { jianguoyunRequestInterval, jianguoyunRetryBase = oldInterval, oldRetry })
	server := httptest.NewServer(fixture)
	t.Cleanup(server.Close)
	svc, db := newTestService(t)
	setCloudConfig(t, db, func(c *store.CloudConfig) {
		c.Enabled, c.Provider, c.URL = true, "jianguoyun", store.JianguoyunBaseURL
		c.Username, c.Password = "test@example.invalid", "test-app-password"
	})
	t.Cleanup(func() { _ = db.DisconnectJianguoyun() })
	svc.Endpoints.JianguoyunDAV = server.URL + "/dav/"
	return svc
}

func TestJianguoyunPresetUsesSeparateFolderAndVerifiesUpload(t *testing.T) {
	fixture := &jianguoyunDAVFixture{}
	svc := newJianguoyunService(t, fixture)
	name := "game__main__snap.zip"
	if err := svc.UploadIfAbsent(writeTempZip(t, "zip bytes"), name); err != nil {
		t.Fatal(err)
	}
	fixture.Lock()
	defer fixture.Unlock()
	if !fixture.folder || string(fixture.objects[name]) != "zip bytes" || fixture.realPuts != 1 {
		t.Fatalf("dedicated folder upload failed: folder=%t realPUTs=%d", fixture.folder, fixture.realPuts)
	}
}

func TestJianguoyunCredentialChangeRequiresFreshConditionProbe(t *testing.T) {
	fixture := &jianguoyunDAVFixture{}
	svc := newJianguoyunService(t, fixture)
	if err := svc.UploadIfAbsent(writeTempZip(t, "first"), "game__main__one.zip"); err != nil {
		t.Fatal(err)
	}
	cfg, err := svc.Store.GetCloudConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Password = "rotated-test-app-password"
	if err := svc.Store.UpdateCloudConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := svc.UploadIfAbsent(writeTempZip(t, "second"), "game__main__two.zip"); err != nil {
		t.Fatal(err)
	}
	fixture.Lock()
	defer fixture.Unlock()
	if fixture.probePuts != 4 {
		t.Fatalf("credential rotation reused an old condition probe: probe PUTs=%d", fixture.probePuts)
	}
}

func TestJianguoyunFailedConditionProbeCleanupStopsUpload(t *testing.T) {
	fixture := &jianguoyunDAVFixture{probeDeleteFail: true}
	svc := newJianguoyunService(t, fixture)
	err := svc.UploadIfAbsent(writeTempZip(t, "zip bytes"), "game__main__snap.zip")
	if !errors.Is(err, ErrJianguoyunCondition) {
		t.Fatalf("failed probe cleanup was not reported: %v", err)
	}
	fixture.Lock()
	defer fixture.Unlock()
	if fixture.realPuts != 0 || svc.jianguoyunProbeOK {
		t.Fatalf("failed cleanup enabled a real upload: PUTs=%d probeOK=%t", fixture.realPuts, svc.jianguoyunProbeOK)
	}
}

func TestJianguoyun500MBBoundaryAndPreflight(t *testing.T) {
	if err := jianguoyunFileSizeError(jianguoyunMaxFileBytes); err != nil {
		t.Fatal(err)
	}
	if err := jianguoyunFileSizeError(jianguoyunMaxFileBytes + 1); err == nil || !strings.Contains(err.Error(), "500MB") {
		t.Fatalf("oversized snapshot error = %v", err)
	}
	fixture := &jianguoyunDAVFixture{}
	svc := newJianguoyunService(t, fixture)
	path := filepath.Join(t.TempDir(), "oversized.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(jianguoyunMaxFileBytes + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := svc.Upload(path, "game__main__oversized.zip"); err == nil {
		t.Fatal("oversized snapshot was uploaded")
	}
	fixture.Lock()
	defer fixture.Unlock()
	if fixture.folder || fixture.realPuts != 0 {
		t.Fatal("oversize check sent a WebDAV request before rejecting")
	}
}

func TestJianguoyunStatusErrorsAreActionableAndSanitized(t *testing.T) {
	for _, status := range []int{401, 403, 404, 412, 429, 507, 500} {
		err := jianguoyunStatusError("上传", status)
		if err == nil || strings.Contains(err.Error(), "test-app-password") {
			t.Fatalf("status %d leaked or lacked an error", status)
		}
		if status == 412 && !errors.Is(err, ErrRemoteSnapshotConflict) {
			t.Fatal("412 must be a conflict")
		}
	}
}

func TestJianguoyunWebDAVHTTPFailuresDoNotEchoResponseBodies(t *testing.T) {
	oldInterval, oldRetry := jianguoyunRequestInterval, jianguoyunRetryBase
	jianguoyunRequestInterval, jianguoyunRetryBase = 0, 0
	defer func() { jianguoyunRequestInterval, jianguoyunRetryBase = oldInterval, oldRetry }()
	for _, status := range []int{401, 403, 404, 412, 429, 507} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				fmt.Fprint(w, "private-mail@example.invalid Authorization: private-token")
			}))
			defer server.Close()
			svc := &Service{HTTP: server.Client()}
			err := svc.ensureJianguoyunFolder(store.CloudConfig{URL: server.URL + "/dav/GameSaveGo/", Username: "private-mail@example.invalid", Password: "private-token"})
			if err == nil || strings.Contains(err.Error(), "private-mail") || strings.Contains(err.Error(), "private-token") {
				t.Fatalf("status %d lacked a sanitized error: %v", status, err)
			}
		})
	}
}

func TestJianguoyunFolderCreationRechecksConcurrentWinner(t *testing.T) {
	oldInterval := jianguoyunRequestInterval
	jianguoyunRequestInterval = 0
	defer func() { jianguoyunRequestInterval = oldInterval }()
	checks := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "MKCOL" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		checks++
		if checks == 1 {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusMultiStatus)
		}
	}))
	defer server.Close()
	svc := &Service{HTTP: server.Client()}
	if err := svc.ensureJianguoyunFolder(store.CloudConfig{URL: server.URL + "/dav/GameSaveGo/"}); err != nil || checks != 2 {
		t.Fatalf("concurrent MKCOL result was not rechecked: checks=%d err=%v", checks, err)
	}
}

func TestJianguoyunFullOrMalformedInventoryFailsClosed(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		xml    string
		header string
	}{
		{"full-page", fullJianguoyunPage(), ""},
		{"page-marker", `<D:multistatus xmlns:D="DAV:"><D:response><D:href>/dav/GameSaveGo/</D:href></D:response><D:next>cursor</D:next></D:multistatus>`, ""},
		{"page-header", `<D:multistatus xmlns:D="DAV:"><D:response><D:href>/dav/GameSaveGo/</D:href></D:response></D:multistatus>`, `</dav/GameSaveGo/?page=2>; rel="next"`},
		{"duplicate", `<D:multistatus xmlns:D="DAV:"><D:response><D:href>/dav/GameSaveGo/</D:href></D:response><D:response><D:href>/dav/GameSaveGo/a.zip</D:href></D:response><D:response><D:href>/dav/GameSaveGo/a.zip</D:href></D:response></D:multistatus>`, ""},
		{"truncated", `<D:multistatus xmlns:D="DAV:"><D:response>`, ""},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := &jianguoyunDAVFixture{folder: true, listXML: scenario.xml, listNextHeader: scenario.header}
			svc := newJianguoyunService(t, fixture)
			if files, err := svc.List(); err == nil || files != nil {
				t.Fatalf("incomplete inventory accepted: files=%d err=%v", len(files), err)
			}
			if err := svc.UploadIfAbsent(writeTempZip(t, "new"), "game__main__snap.zip"); err == nil {
				t.Fatal("incomplete inventory allowed an upload")
			}
		})
	}
}

func fullJianguoyunPage() string {
	var xml strings.Builder
	xml.WriteString(`<D:multistatus xmlns:D="DAV:"><D:response><D:href>/dav/GameSaveGo/</D:href></D:response>`)
	for i := 0; i < 749; i++ {
		fmt.Fprintf(&xml, `<D:response><D:href>/dav/GameSaveGo/%d.zip</D:href></D:response>`, i)
	}
	xml.WriteString(`</D:multistatus>`)
	return xml.String()
}

func TestJianguoyunIgnoredConditionAndLengthMismatchFail(t *testing.T) {
	for _, scenario := range []struct {
		name        string
		ignored     bool
		wrongLength bool
		interrupted bool
	}{
		{"ignored-condition", true, false, false},
		{"wrong-length", false, true, false},
		{"interrupted-put", false, false, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := &jianguoyunDAVFixture{ignoreCondition: scenario.ignored, wrongHead: scenario.wrongLength, interruptPUT: scenario.interrupted}
			svc := newJianguoyunService(t, fixture)
			if err := svc.UploadIfAbsent(writeTempZip(t, "original"), "game__main__snap.zip"); err == nil {
				t.Fatal("unsafe upload was reported as complete")
			}
			fixture.Lock()
			defer fixture.Unlock()
			if scenario.name == "ignored-condition" && fixture.realPuts != 0 {
				t.Fatal("real snapshot was sent after server ignored the condition")
			}
			if scenario.name == "interrupted-put" && fixture.realPuts != 1 {
				t.Fatal("uncertain PUT was retried")
			}
		})
	}
}

func TestJianguoyunSameNameConcurrentUploadKeepsFirst(t *testing.T) {
	fixture := &jianguoyunDAVFixture{}
	svc := newJianguoyunService(t, fixture)
	name := "game__main__snap.zip"
	var wg sync.WaitGroup
	errs := make([]error, 2)
	paths := []string{writeTempZip(t, "first"), writeTempZip(t, "second")}
	for i, filePath := range paths {
		wg.Add(1)
		go func(i int, filePath string) {
			defer wg.Done()
			errs[i] = svc.UploadIfAbsent(filePath, name)
		}(i, filePath)
	}
	wg.Wait()
	successes, conflicts := 0, 0
	for _, err := range errs {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrRemoteSnapshotConflict) {
			conflicts++
		} else {
			t.Fatalf("unexpected concurrent result: %v", err)
		}
	}
	fixture.Lock()
	defer fixture.Unlock()
	if successes != 1 || conflicts != 1 || fixture.realPuts != 1 || len(fixture.objects[name]) == 0 {
		t.Fatalf("concurrent upload was not create-only: success=%d conflict=%d realPUTs=%d", successes, conflicts, fixture.realPuts)
	}
}

func TestJianguoyunRetryIsBounded(t *testing.T) {
	oldInterval, oldRetry := jianguoyunRequestInterval, jianguoyunRetryBase
	jianguoyunRequestInterval, jianguoyunRetryBase = 0, 0
	defer func() { jianguoyunRequestInterval, jianguoyunRetryBase = oldInterval, oldRetry }()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	svc := &Service{HTTP: server.Client()}
	req, _ := http.NewRequest("PROPFIND", server.URL, nil)
	resp, err := svc.jianguoyunDo(req, true)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 429 || requests != 3 {
		t.Fatalf("retry count=%d status=%d", requests, resp.StatusCode)
	}
}

func TestJianguoyunInterruptedDownloadNeverPublishes(t *testing.T) {
	fixture := &jianguoyunDAVFixture{folder: true, objects: map[string][]byte{
		"game__main__snap.zip": testZIP(t, "save.dat", "progress"),
	}, interruptGET: true}
	svc := newJianguoyunService(t, fixture)
	dir := t.TempDir()
	file := CloudFile{Name: "game__main__snap.zip", SizeBytes: int64(len(fixture.objects["game__main__snap.zip"]))}
	dest := filepath.Join(dir, "published.zip")
	if err := svc.DownloadVerified(file, dest, ""); err == nil {
		t.Fatal("truncated WebDAV response was published")
	}
	if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed download published a local file: %v", err)
	}
	assertNoPartFiles(t, dir)
}
