package pim

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const roleDefCacheTTL = 24 * time.Hour

type roleDefCacheFile struct {
	FetchedAt time.Time          `json:"fetched_at"`
	Entries   []roleDefEntryJSON `json:"entries"`
}

type roleDefEntryJSON struct {
	RoleDefID   string `json:"role_def_id"`
	DisplayName string `json:"display_name"`
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
	m := make(map[string]string, len(cf.Entries))
	for _, e := range cf.Entries {
		m[e.RoleDefID] = e.DisplayName
	}
	return m
}

// saveRoleDefDiskCache writes role definition names to a cache file.
// Failures are silently ignored.
func saveRoleDefDiskCache(path string, m map[string]string) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return
	}
	entries := make([]roleDefEntryJSON, 0, len(m))
	for id, name := range m {
		entries = append(entries, roleDefEntryJSON{RoleDefID: id, DisplayName: name})
	}
	data, err := json.Marshal(roleDefCacheFile{FetchedAt: time.Now(), Entries: entries})
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0600)
}
