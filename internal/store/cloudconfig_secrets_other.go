//go:build !windows

package store

// Keep the existing non-Windows storage behavior until a protected keyring is
// available there. The Windows desktop release never falls back to this path.
func (s *Store) loadAdditionalCloudSecrets(c *CloudConfig) error {
	if c.Provider == "webdav" {
		c.PasswordConfigured = c.Password != ""
	}
	c.HeadersConfigured = nonemptyHeaders(c.HeadersJSON)
	return nil
}

func (s *Store) loadAdditionalCloudPassword(c CloudConfig) (string, error) { return c.Password, nil }

func (s *Store) protectAdditionalCloudSecrets(_ *CloudConfig, writeRow func() error) error {
	return writeRow()
}

func nonemptyHeaders(raw string) bool { return raw != "" && raw != "{}" }
