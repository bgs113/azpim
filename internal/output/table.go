package output

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"azpim/internal/pim"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
)

// ANSI color codes (only applied when stdout is a terminal).
const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorCyan   = "\033[36m"
)

func colorize(text, color string) string {
	if fileInfo, _ := os.Stdout.Stat(); (fileInfo.Mode() & os.ModeCharDevice) == 0 {
		return text
	}
	return color + text + colorReset
}

func stateColor(state string) string {
	switch strings.ToLower(state) {
	case "active":
		return colorize(state, colorGreen)
	case "permanent":
		return colorize(state, colorCyan)
	case "pending":
		return colorize(state, colorYellow)
	case "failed", "expired":
		return colorize(state, colorRed)
	default:
		return state
	}
}

// borderlessRendition returns a Rendition with no outer borders but a header separator.
func borderlessRendition() tw.Rendition {
	return tw.Rendition{
		Borders: tw.Border{
			Left:   tw.Off,
			Right:  tw.Off,
			Top:    tw.Off,
			Bottom: tw.Off,
		},
		Settings: tw.Settings{
			Separators: tw.Separators{
				BetweenRows:    tw.Off,
				BetweenColumns: tw.On,
			},
			Lines: tw.Lines{
				ShowHeaderLine: tw.On,
			},
		},
	}
}

// PrintEligibleTable writes eligible assignments as an aligned table to w.
func PrintEligibleTable(w io.Writer, assignments []pim.EligibleAssignment) {
	if len(assignments) == 0 {
		fmt.Fprintln(w, "No eligible assignments found.")
		return
	}

	t := tablewriter.NewTable(w,
		tablewriter.WithRenderer(renderer.NewBlueprint(borderlessRendition())),
		tablewriter.WithHeaderAlignment(tw.AlignLeft),
		tablewriter.WithRowAlignment(tw.AlignLeft),
	)
	t.Header("ROLE", "SCOPE", "RESOURCE TYPE", "MEMBERSHIP", "CONDITION", "END TIME")

	for _, a := range assignments {
		end := "-"
		if a.HasExpiry {
			end = formatTime(a.EndTime)
		}
		cond := truncate(a.Condition, 40)
		if cond == "" {
			cond = "-"
		}
		if err := t.Append([]string{
			a.RoleName,
			a.ScopeDisplay,
			a.ResourceType,
			a.MembershipType,
			cond,
			end,
		}); err != nil {
			fmt.Fprintf(w, "error appending row: %v\n", err)
		}
	}
	if err := t.Render(); err != nil {
		fmt.Fprintf(w, "error rendering table: %v\n", err)
	}
}

// PrintActiveTable writes active assignments as an aligned table to w.
// humanReadable controls the format of the Time remaining column.
func PrintActiveTable(w io.Writer, assignments []pim.ActiveAssignment, humanReadable bool) {
	if len(assignments) == 0 {
		fmt.Fprintln(w, "No active assignments found.")
		return
	}

	t := tablewriter.NewTable(w,
		tablewriter.WithRenderer(renderer.NewBlueprint(borderlessRendition())),
		tablewriter.WithHeaderAlignment(tw.AlignLeft),
		tablewriter.WithAlignment(tw.Alignment{
			tw.AlignLeft,  // Role
			tw.AlignLeft,  // Resource
			tw.AlignLeft,  // Resource type
			tw.AlignLeft,  // Membership
			tw.AlignLeft,  // Condition
			tw.AlignLeft,  // State
			tw.AlignLeft,  // End time
			tw.AlignRight, // Time remaining
		}),
	)
	t.Header("ROLE", "RESOURCE", "RESOURCE TYPE", "MEMBERSHIP", "CONDITION", "STATE", "END TIME", "TIME REMAINING")

	for _, a := range assignments {
		end := "-"
		if a.HasExpiry {
			end = formatTime(a.EndTime)
		}
		cond := truncate(a.Condition, 40)
		if cond == "" {
			cond = "-"
		}
		if err := t.Append([]string{
			a.RoleName,
			a.Resource,
			a.ResourceType,
			a.MembershipType,
			cond,
			stateColor(a.State),
			end,
			a.TimeRemaining(humanReadable),
		}); err != nil {
			fmt.Fprintf(w, "error appending row: %v\n", err)
		}
	}
	if err := t.Render(); err != nil {
		fmt.Fprintf(w, "error rendering table: %v\n", err)
	}
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
