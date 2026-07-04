package pim

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRoleDefDiskCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "roledefs.json")

	want := map[string]string{
		"/providers/Microsoft.Authorization/roleDefinitions/abc": "Contributor",
		"/providers/Microsoft.Authorization/roleDefinitions/def": "Owner",
	}

	saveRoleDefDiskCache(path, want)

	got := loadRoleDefDiskCache(path)
	if got == nil {
		t.Fatal("loadRoleDefDiskCache returned nil")
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d", len(got), len(want))
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("got[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestRoleDefDiskCacheExpired(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "roledefs.json")

	// Write a cache file with an old timestamp.
	stale := roleDefCacheFile{
		FetchedAt: time.Now().Add(-25 * time.Hour),
		Entries:   map[string]string{"/providers/foo": "Foo"},
	}
	data, _ := json.Marshal(stale)
	_ = os.WriteFile(path, data, 0600)

	got := loadRoleDefDiskCache(path)
	if got != nil {
		t.Errorf("expected nil for expired cache, got %v", got)
	}
}

func TestRoleDefDiskCacheCorrupt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "roledefs.json")

	_ = os.WriteFile(path, []byte("not valid json {{{"), 0600)

	got := loadRoleDefDiskCache(path)
	if got != nil {
		t.Errorf("expected nil for corrupt cache, got %v", got)
	}
}

func TestRoleDefDiskCacheMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.json")

	got := loadRoleDefDiskCache(path)
	if got != nil {
		t.Errorf("expected nil for missing file, got %v", got)
	}
}
