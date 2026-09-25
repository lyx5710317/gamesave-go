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
	ErrJianguoyunCondition  = errors.New("坚果云服务端未证明同名条件写入安全，已停止备份")
	ErrJianguoyunIntegrity  = errors.New("坚果云上传后大小验证失败，请检查远端对象")
)

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

func (s *Service) verifyJianguoyunConditionalCreate(cfg store.CloudConfig) (result error) {
	s.jianguoyunProbeMu.Lock()
	defer s.jianguoyunProbeMu.Unlock()
	key := sha256.Sum256([]byte(cfg.URL + "\x00" + cfg.Username + "\x00" + cfg.Password))
	if s.jianguoyunProbeOK && s.jianguoyunProbeKey == key {
		return nil
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return errors.New("无法生成坚果云条件写入探针")
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
	first, err := put([]byte("probe-one"))
	if err != nil {
		return err
	}
	if first != http.StatusCreated && first != http.StatusNoContent {
		return jianguoyunStatusError("验证条件写入", first)
	}
	defer func() {
		req, err := http.NewRequest(http.MethodDelete, probeURL, nil)
		if err != nil {
			s.jianguoyunProbeOK = false
			result = errors.Join(result, ErrJianguoyunCondition)
			return
		}
		applyBasicAuth(req, cfg.Username, cfg.Password)
		resp, err := s.jianguoyunDo(req, false)
		if err != nil {
			s.jianguoyunProbeOK = false
			result = errors.Join(result, ErrJianguoyunCondition)
			return
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			s.jianguoyunProbeOK = false
			result = errors.Join(result, ErrJianguoyunCondition)
		}
	}()
	second, err := put([]byte("probe-two"))
	if err != nil {
		return err
	}
	if second != http.StatusPreconditionFailed {
		return ErrJianguoyunCondition
	}
	get, _ := http.NewRequest(http.MethodGet, probeURL, nil)
	applyBasicAuth(get, cfg.Username, cfg.Password)
	resp, err := s.jianguoyunDo(get, true)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32))
	if err != nil || resp.StatusCode != http.StatusOK || !bytes.Equal(body, []byte("probe-one")) {
		return ErrJianguoyunCondition
	}
	s.jianguoyunProbeOK = true
	s.jianguoyunProbeKey = key
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
	if err := s.verifyJianguoyunConditionalCreate(cfg); err != nil {
		return err
	}
	target := joinURL(cfg.URL, url.PathEscape(fileName))
	req, err := http.NewRequest(http.MethodPut, target, file)
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
	head, _ := http.NewRequest(http.MethodHead, target, nil)
	applyBasicAuth(head, cfg.Username, cfg.Password)
	resp, err = s.jianguoyunDo(head, true)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength < 0 || resp.ContentLength != size {
		return fmt.Errorf("%w：期望 %d 字节，远端报告 %d 字节；不要盲目重试", ErrJianguoyunIntegrity, size, resp.ContentLength)
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
