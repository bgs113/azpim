package auth

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// NewCredential returns a DefaultAzureCredential that picks up az login,
// environment variables (AZURE_CLIENT_ID / SECRET / TENANT_ID), workload
// identity, and other standard Azure auth sources automatically.
func NewCredential() (azcore.TokenCredential, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure credential: %w\n\nTip: run 'az login' or 'azd auth login' first", err)
	}
	return cred, nil
}

// ResolveTokenClaim fetches an ARM access token and extracts a named JWT claim
// from the payload. Common claims: "oid" (principal object ID), "tid" (tenant ID).
func ResolveTokenClaim(ctx context.Context, cred azcore.TokenCredential, claim string) (string, error) {
	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://management.azure.com/.default"},
	})
	if err != nil {
		return "", fmt.Errorf("get token: %w", err)
	}

	parts := strings.Split(token.Token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("unexpected token format")
	}

	seg := parts[1]

	payload, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		return "", fmt.Errorf("decode token payload: %w", err)
	}

	value := extractJSONStringValue(string(payload), claim)
	if value == "" {
		return "", fmt.Errorf("%q claim not found in token", claim)
	}
	return value, nil
}

// ResolveTenantID extracts the tenant ID from the active credential's token.
func ResolveTenantID(ctx context.Context, cred azcore.TokenCredential) (string, error) {
	return ResolveTokenClaim(ctx, cred, "tid")
}

// ResolvePrincipalID extracts the user/service-principal object ID from the token.
func ResolvePrincipalID(ctx context.Context, cred azcore.TokenCredential) (string, error) {
	return ResolveTokenClaim(ctx, cred, "oid")
}

// extractJSONStringValue extracts a string value for a key from a flat JSON object.
func extractJSONStringValue(json, key string) string {
	search := `"` + key + `":"`
	idx := strings.Index(json, search)
	if idx < 0 {
		return ""
	}
	start := idx + len(search)
	rest := json[start:]
	before, _, ok := strings.Cut(rest, `"`)
	if !ok {
		return ""
	}
	return before
}
