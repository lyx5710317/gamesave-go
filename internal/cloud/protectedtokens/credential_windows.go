//go:build windows

package protectedtokens

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	credentialTypeGeneric  = 1
	credentialPersistLocal = 2
	maxCredentialBlobBytes = 5 * 512
)

// credentialW mirrors CREDENTIALW in wincred.h. All pointers passed to the
// Windows API stay live for the duration of each call.
type credentialW struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

var (
	credentialDLL  = windows.NewLazySystemDLL("advapi32.dll")
	procCredWrite  = credentialDLL.NewProc("CredWriteW")
	procCredRead   = credentialDLL.NewProc("CredReadW")
	procCredDelete = credentialDLL.NewProc("CredDeleteW")
	procCredFree   = credentialDLL.NewProc("CredFree")
)

func writeCredential(target string, raw []byte) error {
	if len(raw) == 0 || len(raw) > maxCredentialBlobBytes {
		return fmt.Errorf("protected cloud token record exceeds Windows credential size limit")
	}
	name, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return errors.New("invalid Windows credential target")
	}
	user, _ := windows.UTF16PtrFromString("GameSave Go")
	credential := credentialW{
		Type: credentialTypeGeneric, TargetName: name,
		CredentialBlobSize: uint32(len(raw)), CredentialBlob: &raw[0],
		Persist: credentialPersistLocal, UserName: user,
	}
	result, _, callErr := procCredWrite.Call(uintptr(unsafe.Pointer(&credential)), 0)
	runtime.KeepAlive(raw)
	runtime.KeepAlive(name)
	runtime.KeepAlive(user)
	if result == 0 {
		return fmt.Errorf("write Windows credential: %w", winCallErr(callErr))
	}
	return nil
}

func readCredential(target string) ([]byte, error) {
	name, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return nil, errors.New("invalid Windows credential target")
	}
	var credential *credentialW
	result, _, callErr := procCredRead.Call(
		uintptr(unsafe.Pointer(name)), credentialTypeGeneric, 0,
		uintptr(unsafe.Pointer(&credential)),
	)
	runtime.KeepAlive(name)
	if result == 0 {
		if errors.Is(callErr, windows.ERROR_NOT_FOUND) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("read Windows credential: %w", winCallErr(callErr))
	}
	if credential == nil {
		return nil, errors.New("Windows returned an empty credential")
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(credential)))
	if credential.CredentialBlobSize == 0 || credential.CredentialBlobSize > maxCredentialBlobBytes || credential.CredentialBlob == nil {
		return nil, errors.New("Windows credential is empty or invalid")
	}
	secret := unsafe.Slice(credential.CredentialBlob, credential.CredentialBlobSize)
	copyOfSecret := append([]byte(nil), secret...)
	clear(secret)
	return copyOfSecret, nil
}

func deleteCredential(target string) error {
	name, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return errors.New("invalid Windows credential target")
	}
	result, _, callErr := procCredDelete.Call(uintptr(unsafe.Pointer(name)), credentialTypeGeneric, 0)
	runtime.KeepAlive(name)
	if result == 0 && !errors.Is(callErr, windows.ERROR_NOT_FOUND) {
		return fmt.Errorf("delete Windows credential: %w", winCallErr(callErr))
	}
	return nil
}

func winCallErr(err error) error {
	if err == nil || err == syscall.Errno(0) {
		return errors.New("Windows credential operation failed")
	}
	return err
}
