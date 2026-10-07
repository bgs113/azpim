package cmd

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bgs113/azpim/internal/pim"
	"github.com/spf13/cobra"
)

func TestSortParseErrors(t *testing.T) {
	tests := []struct {
		values []string
		want   string
	}{
		{[]string{"remainin"}, `invalid --sort key "remainin" for active: did you mean "remaining"? Valid keys: role, resource, type, membership, state, end, remaining`},
		{[]string{"zzz"}, `invalid --sort key "zzz" for active. Valid keys:`},
		{[]string{"rem"}, `invalid --sort key "rem"`}, // no prefix matching
		{[]string{"role", "Role:desc"}, `--sort key "role" is given more than once`},
		{[]string{"role:down"}, `invalid --sort direction "down" in "role:down": use role:asc or role:desc`},
		{[]string{"role:"}, `invalid --sort direction`},
		{[]string{""}, "empty --sort key"},
	}
	for _, tt := range tests {
		_, err := activeSort.parse(tt.values, "active")
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("parse(%q) err = %v, want it to contain %q", tt.values, err, tt.want)
		}
	}
	terms, err := activeSort.parse([]string{" Role ", "end:DESC", "state:asc"}, "active")
	want := []sortTerm{{key: "role"}, {key: "end", desc: true}, {key: "state"}}
	if err != nil || !slices.Equal(terms, want) {
		t.Errorf("parse = %v, %v; want %v", terms, err, want)
	}
}

// --sort a,b and --sort a --sort b are the same to pflag's StringSlice.
func TestSortFlagCommaOrRepeated(t *testing.T) {
	for _, args := range [][]string{{"--sort", "scope,role:desc"}, {"--sort", "scope", "--sort", "role:desc"}} {
		var f outputFlags
		c := &cobra.Command{Use: "x"}
		addSortFlags(c, &f, eligibleSort)
		if err := c.ParseFlags(args); err != nil {
			t.Fatal(err)
		}
		if want := []string{"scope", "role:desc"}; !slices.Equal(f.Sort, want) {
			t.Errorf("%v: Sort = %q, want %q", args, f.Sort, want)
		}
	}
}

func eligibleRows(rows []pim.EligibleAssignment) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.RoleName + "@" + r.ScopeDisplay
	}
	return out
}

func TestSortEligible(t *testing.T) {
	end := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := func() []pim.EligibleAssignment {
		return []pim.EligibleAssignment{
			{RoleName: "reader", ScopeDisplay: "B", Scope: "/b", EndTime: end, HasExpiry: true},
			{RoleName: "Contributor", ScopeDisplay: "b", Scope: "/b2"},
			{RoleName: "Contributor", ScopeDisplay: "A", Scope: "/a", EndTime: end.Add(time.Hour), HasExpiry: true},
			{RoleName: "Reader", ScopeDisplay: "A", Scope: "/a"},
		}
	}
	tests := []struct {
		sort    []string
		reverse bool
		want    []string
	}{
		// Default: role, scope, ignoring case; the raw scope breaks the B/b tie.
		{nil, false, []string{"Contributor@A", "Contributor@b", "Reader@A", "reader@B"}},
		{nil, true, []string{"reader@B", "Reader@A", "Contributor@b", "Contributor@A"}},
		{[]string{"scope", "role:desc"}, false, []string{"Reader@A", "Contributor@A", "reader@B", "Contributor@b"}},
		// Missing end times go last in both directions.
		{[]string{"end"}, false, []string{"reader@B", "Contributor@A", "Contributor@b", "Reader@A"}},
		{[]string{"end:desc"}, false, []string{"Contributor@A", "reader@B", "Contributor@b", "Reader@A"}},
		{[]string{"end"}, true, []string{"Contributor@A", "reader@B", "Reader@A", "Contributor@b"}},
	}
	for _, tt := range tests {
		terms, err := eligibleSort.parse(tt.sort, "eligible")
		if err != nil {
			t.Fatal(err)
		}
		r := rows()
		eligibleSort.sort(r, terms, tt.reverse)
		if got := eligibleRows(r); !slices.Equal(got, tt.want) {
			t.Errorf("--sort %q reverse=%v: got %v, want %v", tt.sort, tt.reverse, got, tt.want)
		}
	}
}

func TestSortActive(t *testing.T) {
	now := time.Now()
	rows := []pim.ActiveAssignment{
		{RoleName: "Owner", Resource: "A", EndTime: now.Add(2 * time.Hour), HasExpiry: true},
		{RoleName: "Reader", Resource: "A"}, // permanent: no end time
		{RoleName: "Contributor", Resource: "B", EndTime: now.Add(time.Hour), HasExpiry: true},
	}
	activeSort.sort(rows, []sortTerm{{key: "remaining"}}, false)
	var got []string
	for _, r := range rows {
		got = append(got, r.RoleName)
	}
	if want := []string{"Contributor", "Owner", "Reader"}; !slices.Equal(got, want) {
		t.Errorf("--sort remaining: got %v, want %v", got, want)
	}
}

func TestSortRequestsDefaultNewestFirst(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := []pim.ScheduleRequestEntry{
		{RoleName: "Old", RequestedAt: t0, HasRequestedAt: true},
		{RoleName: "Unknown"},
		{RoleName: "New", RequestedAt: t0.Add(time.Hour), HasRequestedAt: true},
	}
	requestsSort.sort(rows, nil, false)
	var got []string
	for _, r := range rows {
		got = append(got, r.RoleName)
	}
	if want := []string{"New", "Old", "Unknown"}; !slices.Equal(got, want) {
		t.Errorf("default: got %v, want %v", got, want)
	}
}

func TestSortCompletion(t *testing.T) {
	c := &cobra.Command{Use: "x"}
	addSortFlags(c, &outputFlags{}, requestsSort)
	complete, _ := c.GetFlagCompletionFunc("sort")
	tests := map[string][]string{
		"":              {"role", "scope", "type", "status", "requested", "expires"},
		"s":             {"scope", "status"},
		"role,":         {"role,scope", "role,type", "role,status", "role,requested", "role,expires"},
		"role:desc,sco": {"role:desc,scope"},
	}
	for in, want := range tests {
		got, dir := complete(c, nil, in)
		if !slices.Equal(got, want) {
			t.Errorf("complete(%q) = %v, want %v", in, got, want)
		}
		if dir&cobra.ShellCompDirectiveNoSpace == 0 {
			t.Errorf("complete(%q): want NoSpace so another key can follow a comma", in)
		}
	}
}

// A bad --sort fails before signing in: the error comes from parse, not
// from credentials or the network.
func TestSortValidatedBeforeAzure(t *testing.T) {
	for _, c := range []struct {
		name  string
		flags *outputFlags
	}{{"eligible", &eligibleOutput}, {"active", &activeOutput}, {"requests", &requestsOutput}} {
		t.Cleanup(func() {
			c.flags.Sort = nil
			cmd, _, _ := rootCmd.Find([]string{c.name})
			cmd.Flags().Lookup("sort").Changed = false
			rootCmd.SetArgs(nil)
		})
		rootCmd.SetArgs([]string{c.name, "--sort", "nope"})
		err := rootCmd.Execute()
		if err == nil || !strings.HasPrefix(err.Error(), `invalid --sort key "nope" for `+c.name) {
			t.Errorf("%s --sort nope: err = %v", c.name, err)
		}
	}
}
