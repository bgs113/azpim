package pim

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
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

func TestClientOptionsSetTryTimeout(t *testing.T) {
	if got := clientOptions(cloud.AzurePublic).Retry.TryTimeout; got != TryTimeout {
		t.Errorf("TryTimeout = %v, want %v", got, TryTimeout)
	}
}

// stall is a transport that never answers; it returns only when the request's
// context ends, as a server that accepts a request and then goes quiet would.
type stall struct{ tries int }

func (s *stall) Do(req *http.Request) (*http.Response, error) {
	s.tries++
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func TestStalledRequestTimesOut(t *testing.T) {
	st := &stall{}
	opts := clientOptions(cloud.AzurePublic)
	opts.Transport = st
	opts.Retry.TryTimeout = 20 * time.Millisecond // short versions of the real settings
	opts.Retry.RetryDelay = time.Millisecond
	opts.Retry.MaxRetryDelay = time.Millisecond
	clients, err := newClients(fakeToken{}, opts)
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	_, err = clients.ListEligible(context.Background(), "/")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("took %v, want the per-attempt timeout to end it quickly", elapsed)
	}
	if st.tries != 4 {
		t.Errorf("tried %d times, want 4 (the first attempt plus the SDK's 3 retries)", st.tries)
	}
}
