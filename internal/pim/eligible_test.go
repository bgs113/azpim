package pim

import "testing"

func newEligible(name, guid, scope, membership string) EligibleAssignment {
	return EligibleAssignment{
		RoleName:       name,
		RoleDefID:      "/providers/Microsoft.Authorization/roleDefinitions/" + guid,
		Scope:          scope,
		MembershipType: membership,
	}
}

func newActive(guid, scope, membership string) ActiveAssignment {
	return ActiveAssignment{
		RoleDefID:      "/providers/Microsoft.Authorization/roleDefinitions/" + guid,
		Scope:          scope,
		MembershipType: membership,
	}
}

func TestDeduplicateEligible(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		if got := deduplicateEligible(nil); len(got) != 0 {
			t.Errorf("got %d, want 0", len(got))
		}
	})

	t.Run("no duplicates unchanged", func(t *testing.T) {
		in := []EligibleAssignment{
			newEligible("Contributor", "guid1", "/subscriptions/sub1", "Direct"),
			newEligible("Owner", "guid2", "/subscriptions/sub1", "Direct"),
		}
		if got := deduplicateEligible(in); len(got) != 2 {
			t.Errorf("got %d, want 2", len(got))
		}
	})

	t.Run("exact duplicate collapsed", func(t *testing.T) {
		a := newEligible("Contributor", "guid1", "/subscriptions/sub1", "Direct")
		if got := deduplicateEligible([]EligibleAssignment{a, a}); len(got) != 1 {
			t.Errorf("got %d, want 1", len(got))
		}
	})

	t.Run("group wins over direct for same role+scope", func(t *testing.T) {
		direct := newEligible("Contributor", "guid1", "/subscriptions/sub1", "Direct")
		group := newEligible("Contributor", "guid1", "/subscriptions/sub1", "Group")
		got := deduplicateEligible([]EligibleAssignment{direct, group})
		if len(got) != 1 {
			t.Fatalf("got %d entries, want 1", len(got))
		}
		if got[0].MembershipType != "Group" {
			t.Errorf("membership = %q, want Group", got[0].MembershipType)
		}
	})

	t.Run("group wins regardless of input order", func(t *testing.T) {
		direct := newEligible("Contributor", "guid1", "/subscriptions/sub1", "Direct")
		group := newEligible("Contributor", "guid1", "/subscriptions/sub1", "Group")
		// Group comes first this time
		got := deduplicateEligible([]EligibleAssignment{group, direct})
		if len(got) != 1 || got[0].MembershipType != "Group" {
			t.Errorf("got %v, want single Group entry", got)
		}
	})

	t.Run("same role at different scopes not deduped", func(t *testing.T) {
		a := newEligible("Contributor", "guid1", "/subscriptions/sub1", "Direct")
		b := newEligible("Contributor", "guid1", "/subscriptions/sub2", "Direct")
		if got := deduplicateEligible([]EligibleAssignment{a, b}); len(got) != 2 {
			t.Errorf("got %d, want 2", len(got))
		}
	})
}

func TestFilterEligibleActive(t *testing.T) {
	contribE := newEligible("Contributor", "guid1", "/subscriptions/sub1", "Direct")
	ownerE := newEligible("Owner", "guid2", "/subscriptions/sub1", "Direct")
	eligible := []EligibleAssignment{contribE, ownerE}

	t.Run("no active — all eligible returned", func(t *testing.T) {
		got := FilterEligibleActive(eligible, nil)
		if len(got) != 2 {
			t.Errorf("got %d, want 2", len(got))
		}
	})

	t.Run("active assignment filters out matching eligible", func(t *testing.T) {
		active := []ActiveAssignment{newActive("guid1", "/subscriptions/sub1", "Direct")}
		got := FilterEligibleActive(eligible, active)
		if len(got) != 1 || got[0].RoleName != "Owner" {
			t.Errorf("got %v, want [Owner]", got)
		}
	})

	t.Run("permanent active does not block activation", func(t *testing.T) {
		active := []ActiveAssignment{newActive("guid1", "/subscriptions/sub1", "Permanent")}
		got := FilterEligibleActive(eligible, active)
		if len(got) != 2 {
			t.Errorf("got %d, want 2 (permanent should not suppress eligible)", len(got))
		}
	})

	t.Run("active at different scope does not filter", func(t *testing.T) {
		active := []ActiveAssignment{newActive("guid1", "/subscriptions/sub2", "Direct")}
		got := FilterEligibleActive(eligible, active)
		if len(got) != 2 {
			t.Errorf("got %d, want 2 (different scope)", len(got))
		}
	})
}

func TestRoleDefGUID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/subscriptions/sub1/providers/Microsoft.Authorization/roleDefinitions/abc-123", "abc-123"},
		{"/providers/Microsoft.Authorization/roleDefinitions/abc-123", "abc-123"},
		{"abc-123", "abc-123"}, // bare GUID — no slash, returned as-is
		{"", ""},
	}
	for _, tt := range tests {
		got := roleDefGUID(tt.input)
		if got != tt.want {
			t.Errorf("roleDefGUID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeScope(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/subscriptions/Sub1", "/subscriptions/sub1"},
		{"/subscriptions/sub1/", "/subscriptions/sub1"},
		{"/SUBSCRIPTIONS/SUB1/", "/subscriptions/sub1"},
		{"/subscriptions/sub1", "/subscriptions/sub1"}, // already normalized
	}
	for _, tt := range tests {
		got := normalizeScope(tt.input)
		if got != tt.want {
			t.Errorf("normalizeScope(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestResourceTypeFromScope(t *testing.T) {
	tests := []struct {
		scope string
		want  string
	}{
		{"/providers/Microsoft.Management/managementGroups/mg1", "Management group"},
		{"/subscriptions/sub1", "Subscription"},
		{"/subscriptions/sub1/resourceGroups/rg1", "Resource group"},
		// resource group + provider path → "Resource" (provider wins)
		{"/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Storage/storageAccounts/sa1", "Resource"},
		// provider path not a management group → "Resource"
		{"/providers/Microsoft.Authorization/roleDefinitions/abc", "Resource"},
		{"", ""},
	}
	for _, tt := range tests {
		got := resourceTypeFromScope(tt.scope)
		if got != tt.want {
			t.Errorf("resourceTypeFromScope(%q) = %q, want %q", tt.scope, got, tt.want)
		}
	}
}
