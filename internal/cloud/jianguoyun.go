package cloud

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/opensave/opensave/internal/store"
)

// The official help page says WebDAV's default single-file maximum is 500 M.
// Use decimal MB as the conservative interpretation until a real account
// contract test establishes the provider's exact boundary.
const jianguoyunMaxFileBytes int64 = 500_000_000

var jianguoyunRequestInterval = 3 * time.Second // 600 requests / 30 minutes on a free account
var jianguoyunRetryBase = time.Second

var (
	ErrJianguoyunNetwork    = errors.New("坚果云网络连接中断，请确认远端对象后再重试")
	ErrJianguoyunAuth       = errors.New("坚果云认证失败：请检查注册邮箱和第三方应用密码，不要填写登录密码")
	ErrJianguoyunPermission = errors.New("坚果云拒绝访问：请检查应用密码权限、账户流量或空间")
	ErrJianguoyunMissing    = errors.New("坚果云远端目录或文件不存在，请检查独立备份目录")
	ErrJianguoyunRateLimit  = errors.New("坚果云请求过于频繁：已达到 WebDAV 访问限制，请稍后重试")
	ErrJianguoyunQuota      = errors.New("坚果云流量或空间不足：请检查账户配额后重试")
	ErrJianguoyunIncomplete = errors.New("坚果云远端清单可能不完整，已停止上传判断")
	ErrJianguoyunCondition  = errors.New("坚果云未通过安全写入验证，已停止上传")
	ErrJianguoyunIntegrity  = errors.New("坚果云上传后内容验证失败，请检查远端对象")
)

type jianguoyunCreateMode uint8

const (
	jianguoyunCreateUnknown jianguoyunCreateMode = iota
	jianguoyunCreateConditional
	jianguoyunCreateMove
)

// Only controlled method/status metadata reaches transfer activity. Never
// expose response bodies, request URLs, account names, or credentials there.
type jianguoyunSafetyFailure struct {
	Check  string
	Status int
}

func (e *jianguoyunSafetyFailure) Error() string {
	label := map[string]string{
		"probe_cleanup":  "测试文件清理失败",
		"move_first":     "首次 WebDAV 移动失败",
		"move_overwrite": "服务端未拒绝覆盖移动",
		"move_contents":  "移动后内容验证失败",
		"stage_name":     "临时文件名未确认空闲",
	}[e.Check]
	if label == "" {
		label = "安全检查失败"
	}
	if e.Status > 0 {
		return fmt.Sprintf("%s：%s（HTTP %d）", ErrJianguoyunCondition, label, e.Status)
	}
	return fmt.Sprintf("%s：%s", ErrJianguoyunCondition, label)
}

func (e *jianguoyunSafetyFailure) Unwrap() error { return ErrJianguoyunCondition }

func jianguoyunStatusError(operation string, status int) error {
	switch status {
	case http.StatusUnauthorized:
		return ErrJianguoyunAuth
	case http.StatusForbidden:
		return ErrJianguoyunPermission
	case http.StatusNotFound:
		return ErrJianguoyunMissing
	case http.StatusPreconditionFailed:
		return ErrRemoteSnapshotConflict
	case http.StatusTooManyRequests:
		return ErrJianguoyunRateLimit
	case http.StatusInsufficientStorage:
		return ErrJianguoyunQuota
	default:
		if status >= 500 {
			return fmt.Errorf("坚果云暂时不可用（HTTP %d），请稍后重试", status)
		}
		return fmt.Errorf("坚果云%s失败（HTTP %d）", operation, status)
	}
}

func (s *Service) waitJianguoyun(ctx context.Context) error {
	if jianguoyunRequestInterval <= 0 {
		return nil
	}
	s.jianguoyunMu.Lock()
	now := time.Now()
	ready := s.jianguoyunNext
	if ready.Before(now) {
		ready = now
	}
	s.jianguoyunNext = ready.Add(jianguoyunRequestInterval)
	s.jianguoyunMu.Unlock()
	timer := time.NewTimer(time.Until(ready))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Only bodyless/idempotent metadata requests are retried. A failed PUT may
// already have created an object; retrying it without verified identity could
// overwrite another writer, so snapshot PUT is deliberately one attempt.
func (s *Service) jianguoyunDo(req *http.Request, retry bool) (*http.Response, error) {
	client := *s.httpClient()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if req.Method == http.MethodPut || req.Method == http.MethodGet {
		client.Timeout = perRequestTransferTimeout
	}
	tries := 1
	if retry {
		tries = 3
	}
	for attempt := 0; attempt < tries; attempt++ {
		if err := s.waitJianguoyun(req.Context()); err != nil {
			return nil, ErrJianguoyunNetwork
		}
		resp, err := client.Do(req)
		if err == nil && (!retry || (resp.StatusCode != 429 && resp.StatusCode < 500) || attempt == tries-1) {
			return resp, nil
		}
		if err == nil && resp.StatusCode == http.StatusTooManyRequests && resp.Header.Get("Retry-After") != "" {
			seconds, parseErr := strconv.Atoi(resp.Header.Get("Retry-After"))
			if parseErr != nil || seconds > 30 {
				return resp, nil // a long/unknown server delay is not ours to shorten
			}
		}
		if resp != nil {
			resp.Body.Close()
		}
		if attempt == tries-1 {
			return nil, ErrJianguoyunNetwork
		}
		backoff := time.Duration(1<<attempt) * jianguoyunRetryBase
		if err == nil {
			if seconds, parseErr := strconv.Atoi(resp.Header.Get("Retry-After")); parseErr == nil && seconds > 0 && seconds <= 30 {
				backoff = time.Duration(seconds) * time.Second
			}
		}
		timer := time.NewTimer(backoff)
		select {
		case <-req.Context().Done():
			timer.Stop()
			return nil, ErrJianguoyunNetwork
		case <-timer.C:
		}
	}
	return nil, ErrJianguoyunNetwork
}

func (s *Service) ensureJianguoyunFolder(cfg store.CloudConfig) error {
	check, err := http.NewRequest("PROPFIND", cfg.URL, nil)
	if err != nil {
		return err
	}
	check.Header.Set("Depth", "0")
	applyBasicAuth(check, cfg.Username, cfg.Password)
	resp, err := s.jianguoyunDo(check, true)
	if err != nil {
		return err
	}
	status := resp.StatusCode
	resp.Body.Close()
	if status == http.StatusMultiStatus || status == http.StatusOK {
		return nil
	}
	if status != http.StatusNotFound {
		return jianguoyunStatusError("检查目录", status)
	}
	create, err := http.NewRequest("MKCOL", cfg.URL, nil)
	if err != nil {
		return err
	}
	applyBasicAuth(create, cfg.Username, cfg.Password)
	resp, err = s.jianguoyunDo(create, false)
	if err != nil {
		return err
	}
	status = resp.StatusCode
	resp.Body.Close()
	if status == http.StatusCreated {
		return nil
	}
	if status == http.StatusMethodNotAllowed || status == http.StatusConflict {
		// A concurrent creator may have won. Recheck rather than assuming it.
		resp, err = s.jianguoyunDo(check, true)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusMultiStatus || resp.StatusCode == http.StatusOK {
			return nil
		}
		status = resp.StatusCode
	}
	return jianguoyunStatusError("创建目录", status)
}

// Probe only harmless, unpredictable objects. A WebDAV server may ignore HTTP
// conditional PUT but honor the standard MOVE Overwrite:F precondition. The
// latter lets backup-only uploads stage under a unique name and publish the
// verified stage without a direct PUT to the canonical snapshot name.
func (s *Service) verifyJianguoyunCreateOnly(cfg store.CloudConfig) (jianguoyunCreateMode, error) {
	s.jianguoyunProbeMu.Lock()
	defer s.jianguoyunProbeMu.Unlock()
	key := sha256.Sum256([]byte(cfg.URL + "\x00" + cfg.Username + "\x00" + cfg.Password))
	if s.jianguoyunProbeOK && s.jianguoyunProbeKey == key {
		return s.jianguoyunProbeMode, nil
	}
	s.jianguoyunProbeOK = false
	s.jianguoyunProbeMode = jianguoyunCreateUnknown
	supported, err := s.probeJianguoyunConditionalCreate(cfg)
	if err != nil {
		return jianguoyunCreateUnknown, err
	}
	mode := jianguoyunCreateConditional
	if !supported {
		if err := s.probeJianguoyunMoveNoOverwrite(cfg); err != nil {
			return jianguoyunCreateUnknown, err
		}
		mode = jianguoyunCreateMove
	}
	s.jianguoyunProbeOK = true
	s.jianguoyunProbeKey = key
	s.jianguoyunProbeMode = mode
	return mode, nil
}

func (s *Service) probeJianguoyunConditionalCreate(cfg store.CloudConfig) (supported bool, result error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return false, errors.New("无法生成坚果云条件写入探针")
	}
	probeURL := joinURL(cfg.URL, ".gamesavego-condition-probe-"+hex.EncodeToString(nonce[:])+".zip")
	put := func(body []byte) (int, error) {
		req, err := http.NewRequest(http.MethodPut, probeURL, bytes.NewReader(body))
		if err != nil {
			return 0, err
		}
		req.Header.Set("If-None-Match", "*")
		req.Header.Set("Content-Type", "application/zip")
		applyBasicAuth(req, cfg.Username, cfg.Password)
		resp, err := s.jianguoyunDo(req, false)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		return resp.StatusCode, nil
	}
	defer func() {
		req, err := http.NewRequest(http.MethodDelete, probeURL, nil)
		if err != nil {
			result = errors.Join(result, &jianguoyunSafetyFailure{Check: "probe_cleanup"})
			return
		}
		applyBasicAuth(req, cfg.Username, cfg.Password)
		resp, err := s.jianguoyunDo(req, false)
		if err != nil {
			result = errors.Join(result, &jianguoyunSafetyFailure{Check: "probe_cleanup"})
			return
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
			result = errors.Join(result, &jianguoyunSafetyFailure{Check: "probe_cleanup", Status: resp.StatusCode})
		}
	}()
	first, err := put([]byte("probe-one"))
	if err != nil {
		return false, err
	}
	if first != http.StatusCreated && first != http.StatusNoContent {
		return false, jianguoyunStatusError("验证条件写入", first)
	}
	second, err := put([]byte("probe-two"))
	if err != nil {
		return false, err
	}
	if second != http.StatusPreconditionFailed {
		if second == http.StatusUnauthorized || second == http.StatusForbidden || second == http.StatusTooManyRequests || second == http.StatusInsufficientStorage || second >= 500 {
			return false, jianguoyunStatusError("验证条件写入", second)
		}
		return false, nil
	}
	get, _ := http.NewRequest(http.MethodGet, probeURL, nil)
	applyBasicAuth(get, cfg.Username, cfg.Password)
	resp, err := s.jianguoyunDo(get, true)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32))
	if err != nil || resp.StatusCode != http.StatusOK || !bytes.Equal(body, []byte("probe-one")) {
		return false, nil
	}
	return true, nil
}

// RFC 4918 requires MOVE with Overwrite:F to refuse an occupied destination.
// Verify the behavior on two disposable objects, including both objects'
// contents after the refused move. This is a compatibility check, not a proof
// of atomicity across real clients; remote-vault multi-device writes stay off.
func (s *Service) probeJianguoyunMoveNoOverwrite(cfg store.CloudConfig) (result error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return fmt.Errorf("%w：无法生成移动探针", ErrJianguoyunCondition)
	}
	stem := "gamesavego-move-probe-" + hex.EncodeToString(nonce[:])
	sourceA := joinURL(cfg.URL, stem+"-a.zip")
	sourceB := joinURL(cfg.URL, stem+"-b.zip")
	target := joinURL(cfg.URL, stem+"-target.zip")
	defer func() {
		for _, objectURL := range []string{sourceA, sourceB, target} {
			if err := s.deleteJianguoyunProbe(cfg, objectURL); err != nil {
				result = errors.Join(result, err)
			}
		}
	}()
	for _, item := range []struct {
		url  string
		body []byte
	}{
		{sourceA, []byte("move-probe-one")},
		{sourceB, []byte("move-probe-two")},
	} {
		status, err := s.putJianguoyunProbe(cfg, item.url, item.body)
		if err != nil {
			return err
		}
		if status != http.StatusCreated && status != http.StatusNoContent {
			return jianguoyunStatusError("验证禁止覆盖移动", status)
		}
	}
	status, err := s.moveJianguoyun(cfg, sourceA, target)
	if err != nil {
		return err
	}
	if status != http.StatusCreated && status != http.StatusNoContent && status != http.StatusOK {
		return &jianguoyunSafetyFailure{Check: "move_first", Status: status}
	}
	if ok, err := s.jianguoyunObjectMatches(cfg, sourceA, nil, http.StatusNotFound); err != nil || !ok {
		return errors.Join(err, &jianguoyunSafetyFailure{Check: "move_contents"})
	}
	status, err = s.moveJianguoyun(cfg, sourceB, target)
	if err != nil {
		return err
	}
	if status != http.StatusPreconditionFailed && status != http.StatusConflict {
		return &jianguoyunSafetyFailure{Check: "move_overwrite", Status: status}
	}
	for _, item := range []struct {
		url  string
		body []byte
	}{
		{target, []byte("move-probe-one")},
		{sourceB, []byte("move-probe-two")},
	} {
		ok, err := s.jianguoyunObjectMatches(cfg, item.url, item.body, http.StatusOK)
		if err != nil || !ok {
			return errors.Join(err, &jianguoyunSafetyFailure{Check: "move_contents"})
		}
	}
	return nil
}

func (s *Service) putJianguoyunProbe(cfg store.CloudConfig, objectURL string, body []byte) (int, error) {
	req, err := http.NewRequest(http.MethodPut, objectURL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("If-None-Match", "*")
	req.Header.Set("Content-Type", "application/zip")
	applyBasicAuth(req, cfg.Username, cfg.Password)
	resp, err := s.jianguoyunDo(req, false)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func (s *Service) moveJianguoyun(cfg store.CloudConfig, sourceURL, targetURL string) (int, error) {
	req, err := http.NewRequest("MOVE", sourceURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Destination", targetURL)
	req.Header.Set("Overwrite", "F")
	applyBasicAuth(req, cfg.Username, cfg.Password)
	resp, err := s.jianguoyunDo(req, false)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func (s *Service) jianguoyunObjectMatches(cfg store.CloudConfig, objectURL string, expected []byte, expectedStatus int) (bool, error) {
	req, err := http.NewRequest(http.MethodGet, objectURL, nil)
	if err != nil {
		return false, err
	}
	applyBasicAuth(req, cfg.Username, cfg.Password)
	resp, err := s.jianguoyunDo(req, true)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != expectedStatus {
		return false, nil
	}
	if expectedStatus != http.StatusOK {
		return true, nil
	}
	actual, err := io.ReadAll(io.LimitReader(resp.Body, int64(len(expected)+1)))
	return err == nil && bytes.Equal(actual, expected), err
}

func (s *Service) deleteJianguoyunProbe(cfg store.CloudConfig, objectURL string) error {
	req, err := http.NewRequest(http.MethodDelete, objectURL, nil)
	if err != nil {
		return &jianguoyunSafetyFailure{Check: "probe_cleanup"}
	}
	applyBasicAuth(req, cfg.Username, cfg.Password)
	resp, err := s.jianguoyunDo(req, false)
	if err != nil {
		return &jianguoyunSafetyFailure{Check: "probe_cleanup"}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		return &jianguoyunSafetyFailure{Check: "probe_cleanup", Status: resp.StatusCode}
	}
	return nil
}

func (s *Service) uploadJianguoyun(cfg store.CloudConfig, file *os.File, fileName string, size int64) error {
	if err := jianguoyunFileSizeError(size); err != nil {
		return err
	}
	if err := s.ensureJianguoyunFolder(cfg); err != nil {
		return err
	}
	files, err := s.listWebDAV(cfg)
	if err != nil {
		return err
	}
	for _, remote := range files {
		if remote.Name == fileName {
			return ErrRemoteSnapshotConflict
		}
	}
	mode, err := s.verifyJianguoyunCreateOnly(cfg)
	if err != nil {
		return err
	}
	target := joinURL(cfg.URL, url.PathEscape(fileName))
	if mode == jianguoyunCreateMove {
		if err := s.uploadJianguoyunByMove(cfg, file, target, size); err != nil {
			return err
		}
	} else if mode == jianguoyunCreateConditional {
		// Give HTTP a bounded reader that cannot close the source file; a
		// subsequent full readback may need it for integrity verification.
		req, err := http.NewRequest(http.MethodPut, target, io.NewSectionReader(file, 0, size))
		if err != nil {
			return err
		}
		req.ContentLength = size
		req.Header.Set("Content-Type", "application/zip")
		req.Header.Set("If-None-Match", "*")
		applyBasicAuth(req, cfg.Username, cfg.Password)
		resp, err := s.jianguoyunDo(req, false)
		if err != nil {
			return err // outcome unknown; never retry an uncertain PUT blindly
		}
		status := resp.StatusCode
		resp.Body.Close()
		if status < 200 || status >= 300 {
			return jianguoyunStatusError("上传", status)
		}
	} else {
		return ErrJianguoyunCondition
	}
	return s.verifyJianguoyunUploadedObject(cfg, file, target, size, "最终对象")
}

func (s *Service) uploadJianguoyunByMove(cfg store.CloudConfig, file *os.File, target string, size int64) (result error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return fmt.Errorf("%w：无法生成临时上传名", ErrJianguoyunCondition)
	}
	stage := joinURL(cfg.URL, "gamesavego-upload-stage-"+hex.EncodeToString(nonce[:])+".zip")
	stageExists := false
	defer func() {
		if stageExists {
			if err := s.deleteJianguoyunProbe(cfg, stage); err != nil {
				result = errors.Join(result, err)
			}
		}
	}()
	check, _ := http.NewRequest(http.MethodHead, stage, nil)
	applyBasicAuth(check, cfg.Username, cfg.Password)
	resp, err := s.jianguoyunDo(check, true)
	if err != nil {
		return err
	}
	status := resp.StatusCode
	resp.Body.Close()
	if status != http.StatusNotFound {
		return &jianguoyunSafetyFailure{Check: "stage_name", Status: status}
	}
	req, err := http.NewRequest(http.MethodPut, stage, io.NewSectionReader(file, 0, size))
	if err != nil {
		return err
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", "application/zip")
	req.Header.Set("If-None-Match", "*")
	applyBasicAuth(req, cfg.Username, cfg.Password)
	stageExists = true // an interrupted PUT may still have created it
	resp, err = s.jianguoyunDo(req, false)
	if err != nil {
		return err
	}
	status = resp.StatusCode
	resp.Body.Close()
	if status < 200 || status >= 300 {
		return jianguoyunStatusError("暂存上传", status)
	}
	if err := s.verifyJianguoyunUploadedObject(cfg, file, stage, size, "暂存对象"); err != nil {
		return err // never publish an unverified stage
	}
	status, err = s.moveJianguoyun(cfg, stage, target)
	if err != nil {
		return err // outcome unknown; never retry the MOVE blindly
	}
	if status == http.StatusPreconditionFailed || status == http.StatusConflict {
		return ErrRemoteSnapshotConflict
	}
	if status != http.StatusCreated && status != http.StatusNoContent && status != http.StatusOK {
		return jianguoyunStatusError("发布暂存对象", status)
	}
	stageExists = false // the probe verified a successful MOVE removes its source
	return nil
}

// Some WebDAV servers may report an absent or inconsistent HEAD length. Only
// accept that case after streaming the entire remote body and comparing its
// byte count and SHA-256 with the local snapshot. A failed GET is not success.
func (s *Service) verifyJianguoyunUploadedObject(cfg store.CloudConfig, file *os.File, objectURL string, size int64, phase string) error {
	head, err := http.NewRequest(http.MethodHead, objectURL, nil)
	if err != nil {
		return err
	}
	applyBasicAuth(head, cfg.Username, cfg.Password)
	resp, err := s.jianguoyunDo(head, true)
	if err != nil {
		return err
	}
	status, reportedSize := resp.StatusCode, resp.ContentLength
	resp.Body.Close()
	if status == http.StatusOK && reportedSize == size {
		return nil
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusTooManyRequests || status == http.StatusInsufficientStorage {
		return jianguoyunStatusError("验证上传", status)
	}
	get, err := http.NewRequest(http.MethodGet, objectURL, nil)
	if err != nil {
		return err
	}
	applyBasicAuth(get, cfg.Username, cfg.Password)
	resp, err = s.jianguoyunDo(get, true)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w：%s HEAD 返回 HTTP %d、%d 字节，GET 返回 HTTP %d；不要盲目重试", ErrJianguoyunIntegrity, phase, status, reportedSize, resp.StatusCode)
	}
	position, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	defer file.Seek(position, io.SeekStart)
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	localHash := sha256.New()
	localSize, err := io.Copy(localHash, io.LimitReader(file, size+1))
	if err != nil || localSize != size {
		return fmt.Errorf("%w：本地快照在上传期间发生变化", ErrJianguoyunIntegrity)
	}
	remoteHash := sha256.New()
	remoteSize, err := io.Copy(remoteHash, io.LimitReader(resp.Body, size+1))
	if err != nil {
		return ErrJianguoyunNetwork
	}
	if remoteSize != size || !bytes.Equal(remoteHash.Sum(nil), localHash.Sum(nil)) {
		return fmt.Errorf("%w：%s HEAD 返回 HTTP %d、%d 字节，GET 实收 %d 字节，期望 %d 字节；不要盲目重试", ErrJianguoyunIntegrity, phase, status, reportedSize, remoteSize, size)
	}
	return nil
}

func jianguoyunFileSizeError(size int64) error {
	if size > jianguoyunMaxFileBytes {
		return fmt.Errorf("坚果云 WebDAV 默认单文件上限为 500MB；此快照为 %d 字节，请改用较大容量提供商或缩小快照", size)
	}
	return nil
}

func (s *Service) fetchJianguoyunToFile(req *http.Request, localPath string) error {
	resp, err := s.jianguoyunDo(req, true)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return jianguoyunStatusError("下载", resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0o777); err != nil {
		return err
	}
	out, err := os.Create(localPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		os.Remove(localPath)
		return ErrJianguoyunNetwork
	}
	if err := out.Close(); err != nil {
		os.Remove(localPath)
		return err
	}
	return nil
}

func validateJianguoyunHref(baseURL, href string) error {
	base, baseErr := url.Parse(baseURL)
	item, itemErr := url.Parse(strings.TrimSpace(href))
	if baseErr != nil || itemErr != nil || item.User != nil || item.RawQuery != "" || item.Fragment != "" ||
		(item.IsAbs() && (item.Scheme != base.Scheme || item.Host != base.Host)) {
		return errors.New("坚果云远端清单包含异常对象地址，不能用于无冲突判断")
	}
	basePath := strings.TrimSuffix(base.Path, "/")
	if item.Path == basePath || item.Path == basePath+"/" {
		return nil
	}
	if !strings.HasPrefix(item.Path, basePath+"/") || strings.Contains(strings.TrimPrefix(item.Path, basePath+"/"), "/") {
		return errors.New("坚果云远端清单包含目录外或嵌套对象，不能用于无冲突判断")
	}
	return nil
}
