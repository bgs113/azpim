package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
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

// tokenClaims fetches an ARM access token and decodes its JWT payload.
func tokenClaims(ctx context.Context, cred azcore.TokenCredential) (map[string]any, error) {
	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://management.azure.com/.default"},
	})
	if err != nil {
		return nil, fmt.Errorf("get token: %w", err)
	}

	parts := strings.Split(token.Token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("unexpected token format")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode token payload: %w", err)
	}

	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("decode token claims: %w", err)
	}
	return claims, nil
}

// ResolveTokenClaim fetches an ARM access token and extracts a named JWT claim
// from the payload. Common claims: "oid" (principal object ID), "tid" (tenant ID).
func ResolveTokenClaim(ctx context.Context, cred azcore.TokenCredential, claim string) (string, error) {
	claims, err := tokenClaims(ctx, cred)
	if err != nil {
		return "", err
	}
	value, _ := claims[claim].(string)
	if value == "" {
		return "", fmt.Errorf("%q claim not found in token", claim)
	}
	return value, nil
}

// ResolveIdentity describes who the credential acts as, e.g.
// "alice@contoso.com (tenant <tid>)" or "service principal <appid> (tenant <tid>)".
func ResolveIdentity(ctx context.Context, cred azcore.TokenCredential) (string, error) {
	claims, err := tokenClaims(ctx, cred)
	if err != nil {
		return "", err
	}
	return identityLabel(claims), nil
}

// identityLabel picks upn, then appid for app-only tokens, then oid.
// User tokens carry appid too (the client app, e.g. Azure CLI), so appid only
// counts when the token has no "scp" claim, which delegated tokens always have.
func identityLabel(claims map[string]any) string {
	str := func(k string) string { v, _ := claims[k].(string); return v }
	var who string
	switch {
	case str("upn") != "":
		who = str("upn")
	case str("scp") == "" && str("appid") != "":
		who = "service principal " + str("appid")
	default:
		who = "object " + str("oid")
	}
	if tid := str("tid"); tid != "" {
		who += " (tenant " + tid + ")"
	}
	return who
}

// ResolveTenantID extracts the tenant ID from the active credential's token.
func ResolveTenantID(ctx context.Context, cred azcore.TokenCredential) (string, error) {
	return ResolveTokenClaim(ctx, cred, "tid")
}

// ResolvePrincipalID extracts the user/service-principal object ID from the token.
func ResolvePrincipalID(ctx context.Context, cred azcore.TokenCredential) (string, error) {
	return ResolveTokenClaim(ctx, cred, "oid")
}
