package pim

import (
	"fmt"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
)

// Clients bundles the ARM authorization clients needed for PIM operations.
type Clients struct {
	EligibleInstances *armauthorization.RoleEligibilityScheduleInstancesClient
	ActiveInstances   *armauthorization.RoleAssignmentScheduleInstancesClient
	Requests          *armauthorization.RoleAssignmentScheduleRequestsClient
	PolicyAssignments *armauthorization.RoleManagementPolicyAssignmentsClient
	Policies          *armauthorization.RoleManagementPoliciesClient
	Subscriptions     *armsubscriptions.Client
	cred              azcore.TokenCredential
	mu                sync.Mutex
	// policyCache caches FetchMaxActivationDuration results keyed by "roleGUID|scope".
	policyCache map[string]time.Duration
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

	policyAssignments, err := armauthorization.NewRoleManagementPolicyAssignmentsClient(cred, &opts)
	if err != nil {
		return nil, fmt.Errorf("create policy assignments client: %w", err)
	}

	policies, err := armauthorization.NewRoleManagementPoliciesClient(cred, &opts)
	if err != nil {
		return nil, fmt.Errorf("create policies client: %w", err)
	}

	subscriptions, err := armsubscriptions.NewClient(cred, &opts)
	if err != nil {
		return nil, fmt.Errorf("create subscriptions client: %w", err)
	}

	return &Clients{
		EligibleInstances: eligible,
		ActiveInstances:   active,
		Requests:          requests,
		PolicyAssignments: policyAssignments,
		Policies:          policies,
		Subscriptions:     subscriptions,
		cred:              cred,
		policyCache:       make(map[string]time.Duration),
	}, nil
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
