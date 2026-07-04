package pim

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const roleDefCacheTTL = 24 * time.Hour

type roleDefCacheFile struct {
	FetchedAt time.Time         `json:"fetched_at"`
	Entries   map[string]string `json:"entries"`
}

func roleDefCachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "azpim", "roledefs.json"), nil
}

// loadRoleDefDiskCache reads role definition names from a cache file.
// Returns nil if the file is missing, corrupt, or older than roleDefCacheTTL.
func loadRoleDefDiskCache(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cf roleDefCacheFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil
	}
	if time.Since(cf.FetchedAt) > roleDefCacheTTL {
		return nil
	}
	return cf.Entries
}

// saveRoleDefDiskCache writes role definition names to a cache file.
// Failures are silently ignored.
func saveRoleDefDiskCache(path string, m map[string]string) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return
	}
	data, err := json.Marshal(roleDefCacheFile{FetchedAt: time.Now(), Entries: m})
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0600)
}
