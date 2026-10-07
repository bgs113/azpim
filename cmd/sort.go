package cmd

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/bgs113/azpim/internal/pim"

	"github.com/spf13/cobra"
)

// sortKey is one --sort column. Text columns compare ignoring case; time
// columns compare by time, and rows without one (ok false) sort last in
// either direction.
type sortKey[T any] struct {
	name string
	text func(T) string
	time func(T) (t time.Time, ok bool)
}

// sortTerm is one parsed --sort key and its direction.
type sortTerm struct {
	key  string
	desc bool
}

// sortSpec is what a list command can sort by: its keys, in help order, its
// default order, and a final tiebreaker that makes the order deterministic.
type sortSpec[T any] struct {
	keys     []sortKey[T]
	defaults []sortTerm
	last     func(T) string
}

var eligibleSort = sortSpec[pim.EligibleAssignment]{
	keys: []sortKey[pim.EligibleAssignment]{
		{name: "role", text: func(a pim.EligibleAssignment) string { return a.RoleName }},
		{name: "scope", text: func(a pim.EligibleAssignment) string { return a.ScopeDisplay }},
		{name: "type", text: func(a pim.EligibleAssignment) string { return a.ResourceType }},
		{name: "membership", text: func(a pim.EligibleAssignment) string { return a.MembershipType }},
		{name: "end", time: func(a pim.EligibleAssignment) (time.Time, bool) { return a.EndTime, a.HasExpiry }},
	},
	defaults: []sortTerm{{key: "role"}, {key: "scope"}},
	last:     func(a pim.EligibleAssignment) string { return a.Scope },
}

// activeEnd is the end time, which also orders TIME REMAINING.
func activeEnd(a pim.ActiveAssignment) (time.Time, bool) { return a.EndTime, a.HasExpiry }

var activeSort = sortSpec[pim.ActiveAssignment]{
	keys: []sortKey[pim.ActiveAssignment]{
		{name: "role", text: func(a pim.ActiveAssignment) string { return a.RoleName }},
		{name: "resource", text: func(a pim.ActiveAssignment) string { return a.Resource }},
		{name: "type", text: func(a pim.ActiveAssignment) string { return a.ResourceType }},
		{name: "membership", text: func(a pim.ActiveAssignment) string { return a.MembershipType }},
		{name: "state", text: func(a pim.ActiveAssignment) string { return a.State }},
		{name: "end", time: activeEnd},
		{name: "remaining", time: activeEnd},
	},
	defaults: []sortTerm{{key: "role"}, {key: "resource"}},
	last:     func(a pim.ActiveAssignment) string { return a.Scope },
}

var requestsSort = sortSpec[pim.ScheduleRequestEntry]{
	keys: []sortKey[pim.ScheduleRequestEntry]{
		{name: "role", text: func(r pim.ScheduleRequestEntry) string { return r.RoleName }},
		{name: "scope", text: func(r pim.ScheduleRequestEntry) string { return r.ScopeDisplay }},
		{name: "type", text: func(r pim.ScheduleRequestEntry) string { return r.RequestType }},
		{name: "status", text: func(r pim.ScheduleRequestEntry) string { return r.Status }},
		{name: "requested", time: func(r pim.ScheduleRequestEntry) (time.Time, bool) { return r.RequestedAt, r.HasRequestedAt }},
		{name: "expires", time: func(r pim.ScheduleRequestEntry) (time.Time, bool) { return r.ExpiresAt, r.HasExpiry }},
	},
	defaults: []sortTerm{{key: "requested", desc: true}, {key: "role"}, {key: "scope"}},
	last:     func(r pim.ScheduleRequestEntry) string { return r.Scope },
}

func (s sortSpec[T]) names() []string {
	names := make([]string, len(s.keys))
	for i, k := range s.keys {
		names[i] = k.name
	}
	return names
}

func (s sortSpec[T]) key(name string) sortKey[T] {
	i := slices.IndexFunc(s.keys, func(k sortKey[T]) bool { return k.name == name })
	return s.keys[i]
}

// parse checks --sort values: keys of s, each used once, each optionally
// followed by :asc or :desc. cmdName is for the error messages.
func (s sortSpec[T]) parse(values []string, cmdName string) ([]sortTerm, error) {
	names := s.names()
	valid := "Valid keys: " + strings.Join(names, ", ")
	var terms []sortTerm
	for _, v := range values {
		name, dir, hasDir := strings.Cut(strings.TrimSpace(v), ":")
		name = strings.ToLower(name)
		if name == "" {
			return nil, fmt.Errorf("empty --sort key for %s. %s", cmdName, valid)
		}
		if !slices.Contains(names, name) {
			if guess := closest(name, names); guess != "" {
				return nil, fmt.Errorf("invalid --sort key %q for %s: did you mean %q? %s", name, cmdName, guess, valid)
			}
			return nil, fmt.Errorf("invalid --sort key %q for %s. %s", name, cmdName, valid)
		}
		if slices.ContainsFunc(terms, func(t sortTerm) bool { return t.key == name }) {
			return nil, fmt.Errorf("--sort key %q is given more than once", name)
		}
		t := sortTerm{key: name}
		switch strings.ToLower(dir) {
		case "asc":
		case "desc":
			t.desc = true
		default:
			if hasDir {
				return nil, fmt.Errorf("invalid --sort direction %q in %q: use %s:asc or %s:desc", dir, v, name, name)
			}
		}
		terms = append(terms, t)
	}
	return terms, nil
}

// sort orders rows by terms, then the default order, then s.last. reverse
// flips every direction; rows missing a time still sort last.
func (s sortSpec[T]) sort(rows []T, terms []sortTerm, reverse bool) {
	all := append(slices.Clone(terms), s.defaults...)
	slices.SortStableFunc(rows, func(a, b T) int {
		for _, t := range all {
			if c := s.key(t.key).compare(a, b, t.desc != reverse); c != 0 {
				return c
			}
		}
		c := strings.Compare(s.last(a), s.last(b))
		if reverse {
			c = -c
		}
		return c
	})
}

func (k sortKey[T]) compare(a, b T, desc bool) int {
	var c int
	if k.time != nil {
		ta, oka := k.time(a)
		tb, okb := k.time(b)
		switch {
		case oka != okb && oka:
			return -1 // missing times last, whatever the direction
		case oka != okb:
			return 1
		case !oka:
			return 0
		}
		c = ta.Compare(tb)
	} else {
		c = strings.Compare(strings.ToLower(k.text(a)), strings.ToLower(k.text(b)))
	}
	if desc {
		c = -c
	}
	return c
}

// closest returns the name within two edits of s, or "" if none is.
func closest(s string, names []string) string {
	best, bestDist := "", 3
	for _, n := range names {
		if d := editDistance(s, n); d < bestDist {
			best, bestDist = n, d
		}
	}
	return best
}

// editDistance is the Levenshtein distance between a and b.
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

// addSortFlags registers --sort and --reverse, with shell completion for
// the keys of spec, also after a comma.
func addSortFlags[T any](cmd *cobra.Command, flags *outputFlags, spec sortSpec[T]) {
	names := spec.names()
	cmd.Flags().StringSliceVar(&flags.Sort, "sort", nil,
		fmt.Sprintf("Sort by these columns, comma-separated: %s. Add :desc to sort a column descending", strings.Join(names, ", ")))
	cmd.Flags().BoolVar(&flags.Reverse, "reverse", false, "Reverse the sort order")
	_ = cmd.RegisterFlagCompletionFunc("sort", func(_ *cobra.Command, _ []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		i := strings.LastIndex(toComplete, ",")
		done, partial := toComplete[:i+1], toComplete[i+1:]
		used := map[string]bool{}
		for u := range strings.SplitSeq(done, ",") {
			used[strings.ToLower(strings.SplitN(u, ":", 2)[0])] = true
		}
		var out []cobra.Completion
		for _, n := range names {
			if !used[n] && strings.HasPrefix(n, strings.ToLower(partial)) {
				out = append(out, done+n)
			}
		}
		return out, cobra.ShellCompDirectiveNoSpace | cobra.ShellCompDirectiveNoFileComp
	})
}
