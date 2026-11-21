package cloud

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

var (
	errDriveSessionUnavailable = errors.New("could not confirm the Google Drive upload session")
	errDriveSessionExpired     = errors.New("Google Drive upload session expired")
)

// uploadDriveResumable reserves one ID for the whole operation. A restart
// can only create that ID again, so a lost reply cannot create a second copy
// as part of this operation. The caller still checks the complete listing.
func (s *Service) uploadDriveResumable(token, folderID, fileName string, f *os.File, size int64) (string, error) {
	if size <= 0 || driveChunkSize <= 0 {
		return "", errors.New("Google Drive snapshot has no uploadable content")
	}
	digest := sha256.New()
	if n, err := io.Copy(digest, io.NewSectionReader(f, 0, size)); err != nil || n != size {
		return "", errors.New("could not checksum the local snapshot for Google Drive")
	}
	checksum := fmt.Sprintf("%x", digest.Sum(nil))
	fileID, err := s.reserveDriveFileID(token)
	if err != nil {
		return "", err
	}

	const maxSessions = 2
	for attempt := 0; attempt < maxSessions; attempt++ {
		session, uploadErr := s.startDriveUpload(token, folderID, fileName, fileID, size)
		if uploadErr == nil {
			var completedID string
			completedID, uploadErr = s.uploadDriveSession(session, folderID, fileName, f, size)
			if uploadErr == nil {
				if completedID != fileID {
					return "", fmt.Errorf("Google Drive returned the wrong file ID: %w", errUploadUnconfirmed)
				}
				return fileID, nil
			}
		}
		canRestart := errors.Is(uploadErr, errDriveSessionUnavailable) || errors.Is(uploadErr, errDriveSessionExpired)
		if !canRestart && !errors.Is(uploadErr, errUploadUnconfirmed) && !driveHTTPStatus(uploadErr, http.StatusConflict) {
			return "", uploadErr
		}
		found, err := s.verifyReservedDriveFile(token, fileID, folderID, fileName, size, checksum)
		if err != nil {
			return "", errors.Join(errUploadUnconfirmed, err)
		}
		if found {
			return fileID, nil
		}
		if !canRestart || attempt == maxSessions-1 {
			return "", errors.Join(errUploadUnconfirmed, uploadErr)
		}
	}
	return "", errUploadUnconfirmed
}

func (s *Service) reserveDriveFileID(token string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, s.Endpoints.GoogleAPI+"/drive/v3/files/generateIds?count=1&space=drive&type=files", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	var result struct {
		IDs []string `json:"ids"`
	}
	if err := s.doJSON(req, &result); err != nil {
		return "", err
	}
	if len(result.IDs) != 1 || !validReservedDriveID(result.IDs[0]) {
		return "", errors.New("Google Drive did not reserve one usable file ID")
	}
	return result.IDs[0], nil
}

func validReservedDriveID(id string) bool {
	if id == "" || len(id) > 1024 {
		return false
	}
	for _, ch := range id {
		if !(ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '_' || ch == '-') {
			return false
		}
	}
	return true
}

// verifyReservedDriveFile accepts a recovered result only when Drive confirms
// the reserved ID, original destination and SHA-256 of the local archive.
func (s *Service) verifyReservedDriveFile(token, fileID, folderID, fileName string, size int64, checksum string) (bool, error) {
	query := url.Values{"fields": {"id,name,size,parents,mimeType,trashed,sha256Checksum"}}
	req, err := http.NewRequest(http.MethodGet, s.Endpoints.GoogleAPI+"/drive/v3/files/"+url.PathEscape(fileID)+"?"+query.Encode(), nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	var result struct {
		ID       string   `json:"id"`
		Name     string   `json:"name"`
		Size     string   `json:"size"`
		Parents  []string `json:"parents"`
		MIME     string   `json:"mimeType"`
		Trashed  *bool    `json:"trashed"`
		Checksum string   `json:"sha256Checksum"`
	}
	if err := s.doJSON(req, &result); err != nil {
		if driveHTTPStatus(err, http.StatusNotFound) {
			return false, nil
		}
		return false, errors.New("could not inspect the reserved Google Drive file")
	}
	if result.ID != fileID || result.Name != fileName || result.Size != strconv.FormatInt(size, 10) ||
		len(result.Parents) != 1 || result.Parents[0] != folderID || result.MIME != "application/zip" ||
		result.Trashed == nil || *result.Trashed || !strings.EqualFold(result.Checksum, checksum) {
		return false, errors.New("reserved Google Drive file did not match the snapshot")
	}
	return true, nil
}

func driveHTTPStatus(err error, status int) bool {
	var response *providerHTTPError
	return errors.As(err, &response) && response.status == status
}
