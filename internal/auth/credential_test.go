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
