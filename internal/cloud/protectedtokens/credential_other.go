//go:build !windows

package protectedtokens

func writeCredential(string, []byte) error { return ErrUnsupported }

func readCredential(string) ([]byte, error) { return nil, ErrUnsupported }

func deleteCredential(string) error { return ErrUnsupported }
