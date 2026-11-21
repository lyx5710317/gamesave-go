package cloud

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/opensave/opensave/internal/snapshot"
)

const maxFinishedUploads = 40

var (
	errCloudAuthorization       = errors.New("cloud authorization required")
	errUploadUnconfirmed        = errors.New("cloud upload completion is unconfirmed")
	errIncompleteDriveInventory = errors.New("Drive inventory is incomplete")
)

// UploadRecord describes one upload attempted during this process lifetime.
// It deliberately contains no local path, remote URL, token, or raw provider
// error. It is observability, not a durable retry queue.
type UploadRecord struct {
	ID          uint64 `json:"id"`
	GameID      string `json:"gameId,omitempty"`
	SnapshotID  string `json:"snapshotId,omitempty"`
	Provider    string `json:"provider"`
	Status      string `json:"status"`                // running, succeeded, failed
	Failure     string `json:"failure,omitempty"`     // configuration, conflict, or transfer
	SafetyCheck string `json:"safetyCheck,omitempty"` // fixed diagnostic code, never raw provider data
	HTTPStatus  int    `json:"httpStatus,omitempty"`  // status only, never response body
	StartedAt   string `json:"startedAt"`
	FinishedAt  string `json:"finishedAt,omitempty"`
}

func (s *Service) beginUpload(providerName, fileName string) uint64 {
	gameID, _, snapshotID, ok := snapshot.ParseExportEntryName(fileName)
	if !ok {
		gameID, snapshotID = "", ""
	}
	return s.beginUploadRecord(providerName, gameID, snapshotID)
}

// RecordUploadPreflightFailure records a manual backup that stopped before a
// snapshot upload could start. The error is reduced to the same safe codes.
func (s *Service) RecordUploadPreflightFailure(gameID, snapshotID string, err error) {
	cfg, _ := s.Store.GetCloudConfig()
	s.providersMu.RLock()
	_, known := s.providers[cfg.Provider]
	s.providersMu.RUnlock()
	if !known {
		cfg.Provider = ""
	}
	id := s.beginUploadRecord(cfg.Provider, gameID, snapshotID)
	s.finishUpload(id, err)
}

func (s *Service) beginUploadRecord(providerName, gameID, snapshotID string) uint64 {
	if strings.ContainsAny(gameID+snapshotID, `/\:`) {
		gameID, snapshotID = "", ""
	}
	s.uploadsMu.Lock()
	defer s.uploadsMu.Unlock()
	s.nextUploadID++
	record := UploadRecord{
		ID: s.nextUploadID, GameID: gameID, SnapshotID: snapshotID,
		Provider: providerName, Status: "running", StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s.uploads = append(s.uploads, record)
	return record.ID
}

func (s *Service) finishUpload(id uint64, uploadErr error) {
	s.uploadsMu.Lock()
	defer s.uploadsMu.Unlock()
	for i := range s.uploads {
		if s.uploads[i].ID != id {
			continue
		}
		s.uploads[i].FinishedAt = time.Now().UTC().Format(time.RFC3339)
		if uploadErr == nil {
			s.uploads[i].Status = "succeeded"
		} else {
			s.uploads[i].Status = "failed"
			s.uploads[i].Failure = "transfer"
			if errors.Is(uploadErr, errCloudAuthorization) {
				s.uploads[i].Failure = "authentication"
			} else if IsNotConfigured(uploadErr) {
				s.uploads[i].Failure = "configuration"
			} else if errors.Is(uploadErr, errUploadUnconfirmed) || errors.Is(uploadErr, ErrRemoteSnapshotAmbiguous) {
				s.uploads[i].Failure = "unconfirmed"
			} else if errors.Is(uploadErr, ErrRemoteSnapshotConflict) || errors.Is(uploadErr, os.ErrExist) {
				s.uploads[i].Failure = "conflict"
			} else if errors.Is(uploadErr, ErrJianguoyunAuth) {
				s.uploads[i].Failure = "authentication"
			} else if errors.Is(uploadErr, ErrJianguoyunPermission) {
				s.uploads[i].Failure = "permission"
			} else if errors.Is(uploadErr, ErrJianguoyunQuota) {
				s.uploads[i].Failure = "quota"
			} else if errors.Is(uploadErr, ErrJianguoyunRateLimit) {
				s.uploads[i].Failure = "rate_limit"
			} else if errors.Is(uploadErr, ErrJianguoyunNetwork) {
				s.uploads[i].Failure = "network"
			} else if errors.Is(uploadErr, ErrJianguoyunIncomplete) || errors.Is(uploadErr, errIncompleteDriveInventory) {
				s.uploads[i].Failure = "incomplete_inventory"
			} else if errors.Is(uploadErr, ErrJianguoyunCondition) {
				s.uploads[i].Failure = "unsafe_condition"
				var safety *jianguoyunSafetyFailure
				if errors.As(uploadErr, &safety) {
					s.uploads[i].SafetyCheck = safety.Check
					s.uploads[i].HTTPStatus = safety.Status
				}
			} else if errors.Is(uploadErr, ErrJianguoyunIntegrity) {
				s.uploads[i].Failure = "integrity"
			} else {
				s.uploads[i].Failure = standardUploadFailure(s.uploads[i].Provider, uploadErr)
			}
		}
		break
	}
	// Preserve every in-flight operation, but bound completed history.
	finished := 0
	for _, record := range s.uploads {
		if record.Status != "running" {
			finished++
		}
	}
	if finished <= maxFinishedUploads {
		return
	}
	kept := s.uploads[:0]
	for _, record := range s.uploads {
		if record.Status != "running" && finished > maxFinishedUploads {
			finished--
			continue
		}
		kept = append(kept, record)
	}
	s.uploads = kept
}

func standardUploadFailure(provider string, err error) string {
	var response *providerHTTPError
	if errors.As(err, &response) {
		if provider == "google_drive" && response.status == http.StatusForbidden {
			var detail struct {
				Error struct {
					Errors []struct{ Reason string } `json:"errors"`
				} `json:"error"`
			}
			if json.Unmarshal([]byte(response.response), &detail) == nil {
				for _, entry := range detail.Error.Errors {
					switch entry.Reason {
					case "storageQuotaExceeded":
						return "quota"
					case "rateLimitExceeded", "userRateLimitExceeded":
						return "rate_limit"
					}
				}
			}
		}
		switch response.status {
		case http.StatusUnauthorized:
			return "authentication"
		case http.StatusForbidden:
			return "permission"
		case http.StatusInsufficientStorage:
			return "quota"
		case http.StatusTooManyRequests:
			return "rate_limit"
		}
	}
	var network *net.OpError
	if errors.As(err, &network) {
		return "network"
	}
	return "transfer"
}

// UploadActivity returns newest-first records for the current process only.
func (s *Service) UploadActivity() []UploadRecord {
	s.uploadsMu.Lock()
	defer s.uploadsMu.Unlock()
	result := make([]UploadRecord, len(s.uploads))
	for i, record := range s.uploads {
		result[len(s.uploads)-1-i] = record
	}
	return result
}
