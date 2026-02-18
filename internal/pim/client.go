package pim

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v3"
)

// Clients bundles the ARM authorization clients needed for PIM operations.
type Clients struct {
	EligibleInstances *armauthorization.RoleEligibilityScheduleInstancesClient
	ActiveInstances   *armauthorization.RoleAssignmentScheduleInstancesClient
	Requests          *armauthorization.RoleAssignmentScheduleRequestsClient
	RoleDefinitions   *armauthorization.RoleDefinitionsClient
	PolicyAssignments *armauthorization.RoleManagementPolicyAssignmentsClient
	Policies          *armauthorization.RoleManagementPoliciesClient
	cred              azcore.TokenCredential
	mu             sync.Mutex
	// roleDefCache caches role definition display names keyed by their full ARM ID.
	roleDefCache   map[string]string
	// scopeNameCache caches human-readable display names keyed by ARM scope string.
	scopeNameCache map[string]string
}

// NewClients creates authorization clients. The v3 beta SDK takes no
// subscriptionID in constructors; scope-based routing is done per-call.
func NewClients(cred azcore.TokenCredential) (*Clients, error) {
	opts := arm.ClientOptions{}

	eligible, err := armauthorization.NewRoleEligibilityScheduleInstancesClient(cred, &opts)
	if err != nil {
		return nil, fmt.Errorf("create eligible instances client: %w", err)
	}

	active, err := armauthorization.NewRoleAssignmentScheduleInstancesClient(cred, &opts)
	if err != nil {
		return nil, fmt.Errorf("create active instances client: %w", err)
	}

	requests, err := armauthorization.NewRoleAssignmentScheduleRequestsClient(cred, &opts)
	if err != nil {
		return nil, fmt.Errorf("create schedule requests client: %w", err)
	}

	roleDefs, err := armauthorization.NewRoleDefinitionsClient(cred, &opts)
	if err != nil {
		return nil, fmt.Errorf("create role definitions client: %w", err)
	}

	policyAssignments, err := armauthorization.NewRoleManagementPolicyAssignmentsClient(cred, &opts)
	if err != nil {
		return nil, fmt.Errorf("create policy assignments client: %w", err)
	}

	policies, err := armauthorization.NewRoleManagementPoliciesClient(cred, &opts)
	if err != nil {
		return nil, fmt.Errorf("create policies client: %w", err)
	}

	return &Clients{
		EligibleInstances: eligible,
		ActiveInstances:   active,
		Requests:          requests,
		RoleDefinitions:   roleDefs,
		PolicyAssignments: policyAssignments,
		Policies:          policies,
		cred:              cred,
		roleDefCache:      make(map[string]string),
		scopeNameCache:    make(map[string]string),
	}, nil
}

// ResolveRoleName returns the display name for a role definition ARM ID,
// caching results to avoid redundant API calls.
func (c *Clients) ResolveRoleName(ctx context.Context, roleDefID string) string {
	c.mu.Lock()
	if name, ok := c.roleDefCache[roleDefID]; ok {
		c.mu.Unlock()
		return name
	}
	c.mu.Unlock()

	// roleDefID looks like: /subscriptions/{sub}/providers/Microsoft.Authorization/roleDefinitions/{id}
	// or /providers/Microsoft.Authorization/roleDefinitions/{id} for built-ins.
	// The SDK's Get method requires the scope (everything before /providers/Microsoft.Authorization/roleDefinitions)
	// and the roleDefinitionId (the GUID at the end).
	parts := strings.Split(roleDefID, "/providers/Microsoft.Authorization/roleDefinitions/")
	if len(parts) != 2 {
		return roleDefID
	}
	scope := parts[0]
	if scope == "" {
		scope = "/"
	}
	guid := parts[1]

	resp, err := c.RoleDefinitions.Get(ctx, scope, guid, nil)
	name := guid // fallback to GUID
	if err == nil && resp.Properties != nil && resp.Properties.RoleName != nil && *resp.Properties.RoleName != "" {
		name = *resp.Properties.RoleName
	}

	c.mu.Lock()
	c.roleDefCache[roleDefID] = name
	c.mu.Unlock()
	return name
}

// scopeNameGet reads the scope name cache with the lock held.
func (c *Clients) scopeNameGet(scope string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	name, ok := c.scopeNameCache[scope]
	return name, ok
}

// scopeNameSet writes to the scope name cache with the lock held.
func (c *Clients) scopeNameSet(scope, name string) {
	c.mu.Lock()
	c.scopeNameCache[scope] = name
	c.mu.Unlock()
}

// BuildScope converts convenience flags into an ARM scope string.
//
//	managementGroup: /providers/Microsoft.Management/managementGroups/<id>
//	  special value "/" means tenant root group
//	subscription: /subscriptions/<id>
//	resourceGroup: requires subscription to also be set
//	tenantID: used when managementGroup == "/" to resolve tenant root GUID
func BuildScope(managementGroup, tenantID, subscription, resourceGroup, explicitScope string) (string, error) {
	if explicitScope != "" {
		return explicitScope, nil
	}
	if managementGroup != "" {
		mgID := managementGroup
		if mgID == "/" {
			if tenantID == "" {
				return "", fmt.Errorf("--tenant-root requires --tenant-id to be set (or use --management-group <tenantId>)")
			}
			mgID = tenantID
		}
		return fmt.Sprintf("/providers/Microsoft.Management/managementGroups/%s", mgID), nil
	}
	if subscription != "" {
		if resourceGroup != "" {
			return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", subscription, resourceGroup), nil
		}
		return fmt.Sprintf("/subscriptions/%s", subscription), nil
	}
	return "", fmt.Errorf("specify a scope via --scope, --subscription, --management-group, or --tenant-root")
}

// PtrString safely dereferences a *string, returning "" if nil.
func PtrString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
