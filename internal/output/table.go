package output

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/bgs113/azpim/internal/pim"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// ANSI colors for state and status cells. Lip Gloss drops them when w isn't a
// terminal or NO_COLOR is set.
const (
	green  = lipgloss.Color("2")
	yellow = lipgloss.Color("3")
	red    = lipgloss.Color("1")
	cyan   = lipgloss.Color("6")
)

// newRenderer picks colors for w; tests replace it to force a color profile.
var newRenderer = func(w io.Writer) *lipgloss.Renderer { return lipgloss.NewRenderer(w) }

// column describes one table column. color, if set, picks a cell's color from
// its text; right right-aligns the column.
type column struct {
	header string
	right  bool
	color  func(string) lipgloss.TerminalColor
}

// writeTable writes rows under cols as a table with no outer border, a rule
// under the header and │ between columns. Long cells are not wrapped.
func writeTable(w io.Writer, cols []column, rows [][]string) {
	r := newRenderer(w)
	headers := make([]string, len(cols))
	for i, c := range cols {
		headers[i] = c.header
	}
	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false).
		BorderStyle(r.NewStyle()).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			s := r.NewStyle().Padding(0, 1)
			if cols[col].right && row != table.HeaderRow {
				s = s.Align(lipgloss.Right)
			}
			if c := cols[col].color; c != nil && row != table.HeaderRow {
				if color := c(rows[row][col]); color != nil {
					s = s.Foreground(color)
				}
			}
			return s
		})
	fmt.Fprintln(w, t)
}

func stateColor(state string) lipgloss.TerminalColor {
	switch strings.ToLower(state) {
	case "active":
		return green
	case "permanent":
		return cyan
	case "pending":
		return yellow
	case "failed", "expired":
		return red
	}
	return nil
}

func requestStatusColor(status string) lipgloss.TerminalColor {
	switch strings.ToLower(status) {
	case "active":
		return green
	case "pending", "scheduled":
		return yellow
	case "denied", "failed":
		return red
	}
	return nil
}

// PrintEligibleTable writes eligible assignments as an aligned table to w.
func PrintEligibleTable(w io.Writer, assignments []pim.EligibleAssignment) {
	if len(assignments) == 0 {
		fmt.Fprintln(w, "No eligible assignments found.")
		return
	}

	rows := make([][]string, len(assignments))
	for i, a := range assignments {
		end := "-"
		if a.HasExpiry {
			end = formatTime(a.EndTime)
		}
		rows[i] = []string{a.RoleName, a.ScopeDisplay, a.ResourceType, a.MembershipType, orDash(truncate(a.Condition, 40)), end}
	}
	writeTable(w, []column{
		{header: "ROLE"}, {header: "SCOPE"}, {header: "RESOURCE TYPE"}, {header: "MEMBERSHIP"}, {header: "CONDITION"}, {header: "END TIME"},
	}, rows)
}

// PrintActiveTable writes active assignments as an aligned table to w.
// humanReadable controls the format of the Time remaining column.
func PrintActiveTable(w io.Writer, assignments []pim.ActiveAssignment, humanReadable bool) {
	if len(assignments) == 0 {
		fmt.Fprintln(w, "No active assignments found.")
		return
	}

	rows := make([][]string, len(assignments))
	for i, a := range assignments {
		end := "-"
		if a.HasExpiry {
			end = formatTime(a.EndTime)
		}
		rows[i] = []string{a.RoleName, a.Resource, a.ResourceType, a.MembershipType, orDash(truncate(a.Condition, 40)), a.State, end, a.TimeRemaining(humanReadable)}
	}
	writeTable(w, []column{
		{header: "ROLE"}, {header: "RESOURCE"}, {header: "RESOURCE TYPE"}, {header: "MEMBERSHIP"}, {header: "CONDITION"},
		{header: "STATE", color: stateColor}, {header: "END TIME"}, {header: "TIME REMAINING", right: true},
	}, rows)
}

// PrintRequestsTable writes schedule requests as an aligned table to w.
// If pendingOnly is true, only requests with status "Pending" are shown.
func PrintRequestsTable(w io.Writer, requests []pim.ScheduleRequestEntry, pendingOnly bool) {
	var rows [][]string
	for _, r := range requests {
		if pendingOnly && r.Status != "Pending" {
			continue
		}
		requested := "-"
		if r.HasRequestedAt {
			requested = formatTime(r.RequestedAt)
		}
		expires := "-"
		if r.HasExpiry {
			expires = formatTime(r.ExpiresAt)
		}
		rows = append(rows, []string{r.RoleName, r.ScopeDisplay, r.RequestType, r.Status, requested, expires, orDash(truncate(r.Justification, 40))})
	}

	if len(rows) == 0 {
		if pendingOnly {
			fmt.Fprintln(w, "No pending requests found.")
		} else {
			fmt.Fprintln(w, "No requests found.")
		}
		return
	}

	writeTable(w, []column{
		{header: "ROLE"}, {header: "SCOPE"}, {header: "TYPE"}, {header: "STATUS", color: requestStatusColor},
		{header: "REQUESTED"}, {header: "EXPIRES"}, {header: "JUSTIFICATION"},
	}, rows)
}

// orDash returns s, or "-" when s is empty.
func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04")
}

// truncate shortens s to at most n runes, appending "…" if truncated.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
