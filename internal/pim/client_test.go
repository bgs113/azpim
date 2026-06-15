package pim

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization"
)

func TestBuildScope(t *testing.T) {
	tests := []struct {
		managementGroup string
		tenantID        string
		subscription    string
		resourceGroup   string
		explicitScope   string
		want            string
		wantErr         bool
	}{
		// explicit scope takes priority over everything else
		{"", "", "", "", "/subscriptions/abc", "/subscriptions/abc", false},
		// management group by ID
		{"mg-1", "", "", "", "", "/providers/Microsoft.Management/managementGroups/mg-1", false},
		// tenant root ("/") requires tenantID
		{"/", "tenant-123", "", "", "", "/providers/Microsoft.Management/managementGroups/tenant-123", false},
		// tenant root without tenantID → error
		{"/", "", "", "", "", "", true},
		// subscription only
		{"", "", "sub-1", "", "", "/subscriptions/sub-1", false},
		// subscription + resource group
		{"", "", "sub-1", "rg-1", "", "/subscriptions/sub-1/resourceGroups/rg-1", false},
		// nothing set → error
		{"", "", "", "", "", "", true},
	}
	for _, tt := range tests {
		got, err := BuildScope(tt.managementGroup, tt.tenantID, tt.subscription, tt.resourceGroup, tt.explicitScope)
		if (err != nil) != tt.wantErr {
			t.Errorf("BuildScope(%q,%q,%q,%q,%q): err=%v, wantErr=%v",
				tt.managementGroup, tt.tenantID, tt.subscription, tt.resourceGroup, tt.explicitScope, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("BuildScope(...) = %q, want %q", got, tt.want)
		}
	}
}

func TestPtrString(t *testing.T) {
	if got := PtrString(nil); got != "" {
		t.Errorf("PtrString(nil) = %q, want \"\"", got)
	}
	s := "hello"
	if got := PtrString(&s); got != "hello" {
		t.Errorf("PtrString(%q) = %q, want %q", s, got, s)
	}
}

// fakeRoleDefGetter is a test double for the roleDefGetter interface.
type fakeRoleDefGetter struct {
	mu    sync.Mutex
	calls int
	name  string
	delay time.Duration
}

func (f *fakeRoleDefGetter) Get(_ context.Context, _, _ string, _ *armauthorization.RoleDefinitionsClientGetOptions) (armauthorization.RoleDefinitionsClientGetResponse, error) {
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	f.mu.Lock()
	f.calls++
	name := f.name
	f.mu.Unlock()
	props := armauthorization.RoleDefinitionProperties{RoleName: &name}
	return armauthorization.RoleDefinitionsClientGetResponse{
		RoleDefinition: armauthorization.RoleDefinition{Properties: &props},
	}, nil
}

func TestResolveRoleNameCachesResult(t *testing.T) {
	name := "Contributor"
	fake := &fakeRoleDefGetter{name: name}
	c := &Clients{
		RoleDefinitions: fake,
		roleDefCache:    make(map[string]string),
	}
	roleDefID := "/providers/Microsoft.Authorization/roleDefinitions/b24988ac-6180-42a0-ab88-20f7382dd24c"

	got := c.ResolveRoleName(context.Background(), roleDefID)
	if got != name {
		t.Errorf("got %q, want %q", got, name)
	}
	if fake.calls != 1 {
		t.Errorf("expected 1 API call on first lookup, got %d", fake.calls)
	}

	got2 := c.ResolveRoleName(context.Background(), roleDefID)
	if got2 != name {
		t.Errorf("second call got %q, want %q", got2, name)
	}
	if fake.calls != 1 {
		t.Errorf("expected still 1 API call after cache hit, got %d (cache not working)", fake.calls)
	}
}

func TestResolveRoleNameSingleFlight(t *testing.T) {
	const N = 20
	name := "Owner"
	fake := &fakeRoleDefGetter{name: name, delay: 20 * time.Millisecond}
	c := &Clients{
		RoleDefinitions: fake,
		roleDefCache:    make(map[string]string),
	}
	roleDefID := "/providers/Microsoft.Authorization/roleDefinitions/8e3af657-a8ff-443c-a75c-2fe8c4bcb635"

	var wg sync.WaitGroup
	results := make([]string, N)
	for i := range N {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = c.ResolveRoleName(context.Background(), roleDefID)
		}(i)
	}
	wg.Wait()

	for i, r := range results {
		if r != name {
			t.Errorf("results[%d] = %q, want %q", i, r, name)
		}
	}

	fake.mu.Lock()
	calls := fake.calls
	fake.mu.Unlock()

	// singleflight should coalesce concurrent misses; with 20ms delay and 20
	// goroutines, all should arrive before the first returns → exactly 1 call.
	// We allow up to 3 for CI scheduling jitter.
	if calls > 3 {
		t.Errorf("expected ≤3 API calls (singleflight), got %d for %d goroutines", calls, N)
	}
}

func TestSaveRoleDefCache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "roledefs.json")

	c := &Clients{
		roleDefCache: map[string]string{
			"/providers/Microsoft.Authorization/roleDefinitions/abc": "Contributor",
		},
	}

	c.saveRoleDefCacheTo(path)

	got := loadRoleDefDiskCache(path)
	if got == nil {
		t.Fatal("expected cache to be saved, got nil")
	}
	if got["/providers/Microsoft.Authorization/roleDefinitions/abc"] != "Contributor" {
		t.Errorf("unexpected cache content: %v", got)
	}
}
