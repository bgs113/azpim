package output

import (
	"fmt"
	"image/color"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/bgs113/azpim/internal/pim"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

// ANSI colors for state and status cells. colorOutput drops them when w isn't
// a terminal or NO_COLOR is set.
const (
	green  = lipgloss.Green
	yellow = lipgloss.Yellow
	red    = lipgloss.Red
	cyan   = lipgloss.Cyan
)

// colorOutput wraps w so colors are kept, downsampled or stripped to suit it:
// stripped when w isn't a terminal, or NO_COLOR or TERM=dumb is set. Tests
// replace it to force a color profile.
var colorOutput = func(w io.Writer) io.Writer { return colorprofile.NewWriter(w, os.Environ()) }

// termWidth returns w's width in columns when w is a terminal, else 0; tests
// replace it to fake a terminal.
var termWidth = func(w io.Writer) int {
	if f, ok := w.(*os.File); ok && term.IsTerminal(f.Fd()) {
		if cols, _, err := term.GetSize(f.Fd()); err == nil {
			return cols
		}
	}
	return 0
}

// column describes one table column. color, if set, picks a cell's color from
// its text; right right-aligns the column.
type column struct {
	header string
	right  bool
	color  func(string) color.Color
}

// writeTable writes rows under cols as a table with no outer border, a rule
// under the header and │ between columns. When w is a terminal too narrow for
// the table, long cells wrap onto extra lines within their column; otherwise
// every row stays on one line, so piped output keeps one row per line.
func writeTable(w io.Writer, cols []column, rows [][]string) {
	headers := make([]string, len(cols))
	for i, c := range cols {
		headers[i] = c.header
	}
	// In a terminal, fit the columns to its width; cells wrap within them.
	var widths []int
	if tw := termWidth(w); tw > 0 {
		natural, floors := cellWidths(headers, rows)
		widths = fitWidths(natural, floors, tw-(len(cols)-1)) // - the │ separators
	}
	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			s := lipgloss.NewStyle().Padding(0, 1)
			if widths != nil {
				s = s.Width(widths[col])
			}
			if cols[col].right && row != table.HeaderRow {
				s = s.Align(lipgloss.Right)
			}
			if c := cols[col].color; c != nil && row != table.HeaderRow {
				if fg := c(rows[row][col]); fg != nil {
					s = s.Foreground(fg)
				}
			}
			return s
		})
	fmt.Fprintln(colorOutput(w), t)
}

// cellWidths returns each column's natural width, its widest cell or header,
// and its floor, its widest header or single word, so wrapping never splits a
// word or cuts off a header (Lip Gloss truncates headers rather than wrapping
// them). Both include the one-space padding either side.
func cellWidths(headers []string, rows [][]string) (widths, floors []int) {
	widths = make([]int, len(headers))
	floors = make([]int, len(headers))
	for i, h := range headers {
		floors[i] = ansi.StringWidth(h) + 2
		widths[i] = floors[i]
	}
	for _, row := range rows {
		for i, cell := range row {
			widths[i] = max(widths[i], ansi.StringWidth(cell)+2)
			for word := range strings.FieldsSeq(cell) {
				floors[i] = max(floors[i], ansi.StringWidth(word)+2)
			}
		}
	}
	return widths, floors
}

// fitWidths narrows the widest columns, one column at a time, until the widths
// sum to at most total, so short columns keep their full width. No column goes
// below its floor; if the floors alone don't fit, the table stays too wide.
func fitWidths(widths, floors []int, total int) []int {
	w := slices.Clone(widths)
	for sum(w) > total {
		widest := -1
		for i := range w {
			if w[i] > floors[i] && (widest < 0 || w[i] > w[widest]) {
				widest = i
			}
		}
		if widest < 0 {
			break
		}
		w[widest]--
	}
	return w
}

func sum(ns []int) (n int) {
	for _, v := range ns {
		n += v
	}
	return n
}

func stateColor(state string) color.Color {
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

func requestStatusColor(status string) color.Color {
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

// PrintEligibleTable writes eligible assignments as an aligned table to w. When
// there are none it writes nothing to w and says so on msg (stderr), so a
// pipeline reading w sees empty output.
func PrintEligibleTable(w, msg io.Writer, assignments []pim.EligibleAssignment) {
	if len(assignments) == 0 {
		fmt.Fprintln(msg, "No eligible assignments found.")
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
// humanReadable controls the format of the Time remaining column. Like
// PrintEligibleTable, it reports an empty result on msg, not w.
func PrintActiveTable(w, msg io.Writer, assignments []pim.ActiveAssignment, humanReadable bool) {
	if len(assignments) == 0 {
		fmt.Fprintln(msg, "No active assignments found.")
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
// If pendingOnly is true, only requests with status "Pending" are shown. Like
// PrintEligibleTable, it reports an empty result on msg, not w.
func PrintRequestsTable(w, msg io.Writer, requests []pim.ScheduleRequestEntry, pendingOnly bool) {
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
			fmt.Fprintln(msg, "No pending requests found.")
		} else {
			fmt.Fprintln(msg, "No requests found.")
		}
		return
	}

	writeTable(w, []column{
		{header: "ROLE"}, {header: "SCOPE"}, {header: "TYPE"}, {header: "STATUS", color: requestStatusColor},
		{header: "REQUESTED"}, {header: "EXPIRES"}, {header: "JUSTIFICATION"},
	}, rows)
}

// PrintActivationsTable writes one row per role of a multi-role activation.
// Each row is role, scope, duration, status and, for a failure, the error.
func PrintActivationsTable(w io.Writer, rows [][]string) {
	writeTable(w, []column{
		{header: "ROLE"}, {header: "SCOPE"}, {header: "DURATION"}, {header: "STATUS", color: requestStatusColor}, {header: "ERROR"},
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
