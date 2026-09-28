package output

import (
	"bytes"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bgs113/azpim/internal/pim"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		s    string
		n    int
		want string
	}{
		{"hello", 10, "hello"},       // shorter than limit
		{"hello", 5, "hello"},        // exact length — not truncated
		{"hello world", 5, "hello…"}, // over limit — truncated with ellipsis
		{"", 5, ""},                  // empty string
		{"héllo world", 5, "héllo…"}, // multi-byte runes — "héllo" is 5 runes
	}
	for _, tt := range tests {
		got := truncate(tt.s, tt.n)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
		}
	}
}

func TestFormatTime(t *testing.T) {
	t.Run("zero time returns dash", func(t *testing.T) {
		if got := formatTime(time.Time{}); got != "-" {
			t.Errorf("formatTime(zero) = %q, want \"-\"", got)
		}
	})

	t.Run("non-zero time returns non-empty non-dash string", func(t *testing.T) {
		ts := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
		got := formatTime(ts)
		if got == "" || got == "-" {
			t.Errorf("formatTime(%v) = %q, expected formatted timestamp", ts, got)
		}
	})
}

func TestPrintEligibleTableEmpty(t *testing.T) {
	var buf bytes.Buffer
	PrintEligibleTable(&buf, nil)
	if !strings.Contains(buf.String(), "No eligible assignments found.") {
		t.Errorf("expected empty message, got: %q", buf.String())
	}
}

func TestPrintEligibleTableRow(t *testing.T) {
	var buf bytes.Buffer
	assignments := []pim.EligibleAssignment{
		{
			RoleName:       "Contributor",
			ScopeDisplay:   "my-subscription",
			ResourceType:   "Subscription",
			MembershipType: "Direct",
		},
	}
	PrintEligibleTable(&buf, assignments)
	if !strings.Contains(buf.String(), "Contributor") {
		t.Errorf("expected role name in output, got: %q", buf.String())
	}
}

func TestPrintActiveTableEmpty(t *testing.T) {
	var buf bytes.Buffer
	PrintActiveTable(&buf, nil, false)
	if !strings.Contains(buf.String(), "No active assignments found.") {
		t.Errorf("expected empty message, got: %q", buf.String())
	}
}

func TestPrintRequestsTableEmpty(t *testing.T) {
	var buf bytes.Buffer
	PrintRequestsTable(&buf, nil, false)
	if !strings.Contains(buf.String(), "No requests found.") {
		t.Errorf("expected empty message, got: %q", buf.String())
	}
}

func TestPrintRequestsTablePendingEmpty(t *testing.T) {
	var buf bytes.Buffer
	// One Active request; with pendingOnly=true, should show empty message.
	requests := []pim.ScheduleRequestEntry{
		{RoleName: "Contributor", ScopeDisplay: "My Sub", RequestType: "Activate", Status: "Active"},
	}
	PrintRequestsTable(&buf, requests, true)
	if !strings.Contains(buf.String(), "No pending requests found.") {
		t.Errorf("expected pending-empty message, got: %q", buf.String())
	}
}

func TestPrintRequestsTableRow(t *testing.T) {
	var buf bytes.Buffer
	requests := []pim.ScheduleRequestEntry{
		{
			RoleName:      "Owner",
			ScopeDisplay:  "my-subscription",
			RequestType:   "Activate",
			Status:        "Pending",
			Justification: "incident response",
		},
	}
	PrintRequestsTable(&buf, requests, false)
	out := buf.String()
	if !strings.Contains(out, "Owner") {
		t.Errorf("expected role name in output, got: %q", out)
	}
	if !strings.Contains(out, "Activate") {
		t.Errorf("expected request type in output, got: %q", out)
	}
}

func TestPrintRequestsTablePendingFilter(t *testing.T) {
	var buf bytes.Buffer
	requests := []pim.ScheduleRequestEntry{
		{RoleName: "Owner", RequestType: "Activate", Status: "Pending"},
		{RoleName: "Contributor", RequestType: "Activate", Status: "Active"},
	}
	PrintRequestsTable(&buf, requests, true)
	out := buf.String()
	if !strings.Contains(out, "Owner") {
		t.Errorf("expected pending role in output, got: %q", out)
	}
	if strings.Contains(out, "Contributor") {
		t.Errorf("expected active role to be filtered out, got: %q", out)
	}
}

func TestPrintActiveTableRow(t *testing.T) {
	var buf bytes.Buffer
	assignments := []pim.ActiveAssignment{
		{
			RoleName:       "Owner",
			Resource:       "my-subscription",
			ResourceType:   "Subscription",
			MembershipType: "Direct",
			State:          "Active",
			HasExpiry:      true,
			EndTime:        time.Now().Add(4 * time.Hour),
		},
	}
	PrintActiveTable(&buf, assignments, false)
	if !strings.Contains(buf.String(), "Owner") {
		t.Errorf("expected role name in output, got: %q", buf.String())
	}
}

func sampleActive() []pim.ActiveAssignment {
	return []pim.ActiveAssignment{
		{RoleName: "Owner", Resource: "sub-prod", State: "Active", HasExpiry: true, EndTime: time.Now().Add(4 * time.Hour)},
		{RoleName: "Rôle de lecteur 読み取り", Resource: "Tenant Root Group", State: "Permanent"},
	}
}

func TestTablesWithoutTerminalHaveNoEscapes(t *testing.T) {
	var buf bytes.Buffer
	PrintActiveTable(&buf, sampleActive(), false)
	PrintRequestsTable(&buf, []pim.ScheduleRequestEntry{{RoleName: "Owner", Status: "Pending"}}, false)
	if strings.Contains(buf.String(), "\x1b") {
		t.Errorf("output written to a buffer contains ANSI escapes: %q", buf.String())
	}
}

func TestTableColorsStateWhenColorIsOn(t *testing.T) {
	orig := colorOutput
	colorOutput = func(w io.Writer) io.Writer { return &colorprofile.Writer{Forward: w, Profile: colorprofile.ANSI} }
	t.Cleanup(func() { colorOutput = orig })

	var buf bytes.Buffer
	PrintActiveTable(&buf, sampleActive(), false)
	out := buf.String()
	for _, want := range []string{"\x1b[32m", "\x1b[36m"} { // green Active, cyan Permanent
		if !strings.Contains(out, want) {
			t.Errorf("want %q in colored output, got %q", want, out)
		}
	}
}

// Every line has its column separators at the same display columns, including
// rows with accented and double-width characters.
func TestTableColumnsAlign(t *testing.T) {
	var buf bytes.Buffer
	PrintActiveTable(&buf, sampleActive(), false)
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	seps := func(line string) []int {
		var at []int
		col := 0
		for _, r := range line {
			if r == '│' || r == '┼' {
				at = append(at, col)
			}
			col += ansi.StringWidth(string(r))
		}
		return at
	}
	want := seps(lines[0])
	if len(want) != 7 {
		t.Fatalf("header has %d separators, want 7: %q", len(want), lines[0])
	}
	for _, l := range lines[1:] {
		if got := seps(l); !slices.Equal(got, want) {
			t.Errorf("separators at %v, want %v:\n%s\n%s", got, want, lines[0], l)
		}
	}
}

func TestActiveTableRightAlignsTimeRemaining(t *testing.T) {
	var buf bytes.Buffer
	PrintActiveTable(&buf, sampleActive(), true)
	for _, l := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")[2:] {
		last := l[strings.LastIndex(l, "│")+len("│"):]
		if !strings.HasPrefix(last, "  ") || !strings.HasSuffix(last, " ") || strings.HasSuffix(last, "  ") {
			t.Errorf("TIME REMAINING cell %q is not right-aligned", last)
		}
	}
}

func TestTableWrapsOnlyInNarrowTerminal(t *testing.T) {
	orig := termWidth
	t.Cleanup(func() { termWidth = orig })
	rows := []pim.EligibleAssignment{
		{RoleName: "Azure Kubernetes Service RBAC Cluster Admin", ScopeDisplay: "sub-example-prod", ResourceType: "Subscription", MembershipType: "Group"},
		{RoleName: "Azure Kubernetes Service RBAC Cluster User", ScopeDisplay: "sub-example-prod", ResourceType: "Subscription", MembershipType: "Group"},
	}
	render := func(width int) []string {
		termWidth = func(io.Writer) int { return width }
		var buf bytes.Buffer
		PrintEligibleTable(&buf, rows)
		return strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	}

	piped := render(0)
	if len(piped) != 4 {
		t.Fatalf("not a terminal: got %d lines, want header, rule and one line per row:\n%s", len(piped), strings.Join(piped, "\n"))
	}
	if wide := render(500); !slices.Equal(wide, piped) {
		t.Errorf("wide terminal changed the table:\n%s", strings.Join(wide, "\n"))
	}

	narrow := render(90) // the widest words and headers need 84
	out := strings.Join(narrow, "\n")
	for _, l := range narrow {
		if w := ansi.StringWidth(l); w > 90 {
			t.Errorf("line is %d columns wide, want at most 90: %q", w, l)
		}
	}
	if len(narrow) <= len(piped) {
		t.Errorf("narrow terminal: want wrapped rows, got:\n%s", out)
	}
	// Nothing is cut off and no word is split, so the parts that tell the two
	// roles apart survive.
	for _, word := range []string{"Admin", "User", "sub-example-prod"} {
		if !strings.Contains(out, word) {
			t.Errorf("narrow terminal lost %q:\n%s", word, out)
		}
	}
}
