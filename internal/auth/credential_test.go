package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

// fakeCred returns a JWT with the given claims as its payload.
type fakeCred struct{ claims map[string]string }

func (f *fakeCred) GetToken(_ context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error) {
	payload, _ := json.Marshal(f.claims)
	seg := base64.RawURLEncoding.EncodeToString(payload)
	return azcore.AccessToken{Token: "header." + seg + ".sig"}, nil
}

func TestResolveTokenClaim(t *testing.T) {
	cred := &fakeCred{claims: map[string]string{"oid": "abc123", "tid": "tenant1"}}
	ctx := context.Background()

	if got, err := ResolveTokenClaim(ctx, cred, "oid"); err != nil || got != "abc123" {
		t.Errorf("oid: got %q, %v", got, err)
	}
	if got, err := ResolveTokenClaim(ctx, cred, "tid"); err != nil || got != "tenant1" {
		t.Errorf("tid: got %q, %v", got, err)
	}
	if _, err := ResolveTokenClaim(ctx, cred, "missing"); err == nil {
		t.Error("expected error for missing claim")
	}
}

func TestIdentityLabel(t *testing.T) {
	cases := []struct {
		name   string
		claims map[string]any
		want   string
	}{
		{"user", map[string]any{"upn": "alice@contoso.com", "appid": "cli", "scp": "user_impersonation", "oid": "o1", "tid": "t1"}, "alice@contoso.com (tenant t1)"},
		{"service principal", map[string]any{"appid": "app1", "oid": "o1", "tid": "t1"}, "service principal app1 (tenant t1)"},
		{"user without upn", map[string]any{"appid": "cli", "scp": "user_impersonation", "oid": "o1", "tid": "t1"}, "object o1 (tenant t1)"},
		{"oid only", map[string]any{"oid": "o1"}, "object o1"},
	}
	for _, c := range cases {
		if got := identityLabel(c.claims); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
