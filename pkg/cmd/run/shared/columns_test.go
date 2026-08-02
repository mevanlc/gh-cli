package shared

import (
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/text"
	"github.com/stretchr/testify/assert"
)

func TestAutoColumns(t *testing.T) {
	tests := []struct {
		name     string
		maxWidth int
		want     int
	}{
		{name: "no width at all", maxWidth: 0, want: 1},
		{name: "very narrow terminal", maxWidth: 40, want: 1},
		{name: "conventional 80 column terminal", maxWidth: 80, want: 1},
		{name: "just wide enough for a second column", maxWidth: 82, want: 2},
		{name: "typical wide terminal", maxWidth: 120, want: 2},
		{name: "very wide terminal", maxWidth: 160, want: 3},
		{name: "ultrawide terminal is capped", maxWidth: 400, want: maxAutoColumns},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, AutoColumns(tt.maxWidth))
		})
	}
}

func TestRenderColumns(t *testing.T) {
	tests := []struct {
		name     string
		blocks   [][]string
		maxWidth int
		columns  int
		want     string
	}{
		{
			name:     "no blocks",
			blocks:   nil,
			maxWidth: 120,
			columns:  2,
			want:     "",
		},
		{
			name:     "a single column is the plain full-width layout",
			blocks:   [][]string{{"job a", "  step 1"}, {"job b"}},
			maxWidth: 120,
			columns:  1,
			want: strings.Join([]string{
				"job a",
				"  step 1",
				"job b",
			}, "\n"),
		},
		{
			name:     "more columns requested than there are jobs",
			blocks:   [][]string{{"job a", "  step 1"}},
			maxWidth: 120,
			columns:  4,
			want: strings.Join([]string{
				"job a",
				"  step 1",
			}, "\n"),
		},
		{
			name:     "two balanced columns",
			blocks:   [][]string{{"a1", "a2"}, {"b1"}, {"c1", "c2"}, {"d1"}},
			maxWidth: 52,
			columns:  2,
			want: strings.Join([]string{
				"a1" + strings.Repeat(" ", 25) + "c1",
				"a2" + strings.Repeat(" ", 25) + "c2",
				"b1" + strings.Repeat(" ", 25) + "d1",
			}, "\n"),
		},
		{
			name:     "three balanced columns",
			blocks:   [][]string{{"a1", "a2"}, {"b1"}, {"c1", "c2"}, {"d1"}, {"e1", "e2"}, {"f1"}},
			maxWidth: 79,
			columns:  3,
			want: strings.Join([]string{
				"a1" + strings.Repeat(" ", 25) + "c1" + strings.Repeat(" ", 25) + "e1",
				"a2" + strings.Repeat(" ", 25) + "c2" + strings.Repeat(" ", 25) + "e2",
				"b1" + strings.Repeat(" ", 25) + "d1" + strings.Repeat(" ", 25) + "f1",
			}, "\n"),
		},
		{
			name:     "a job is never split across a column boundary",
			blocks:   [][]string{{"a1", "a2", "a3"}, {"b1"}},
			maxWidth: 52,
			columns:  2,
			want: strings.Join([]string{
				"a1" + strings.Repeat(" ", 25) + "b1",
				"a2",
				"a3",
			}, "\n"),
		},
		{
			name:     "lines too wide for their column are truncated",
			blocks:   [][]string{{"abcdefghijklmnopqrstuvwxyz0123"}, {"second"}},
			maxWidth: 52,
			columns:  2,
			want:     "abcdefghijklmnopqrstuv..." + "  " + "second",
		},
		{
			name:     "columns are dropped when the terminal is too narrow",
			blocks:   [][]string{{"a1"}, {"b1"}, {"c1"}},
			maxWidth: 40,
			columns:  3,
			want: strings.Join([]string{
				"a1",
				"b1",
				"c1",
			}, "\n"),
		},
		{
			name: "padding is measured in display width, not bytes",
			blocks: [][]string{
				{"\x1b[32mok\x1b[0m"},
				{"next"},
			},
			maxWidth: 52,
			columns:  2,
			want:     "\x1b[32mok\x1b[0m" + strings.Repeat(" ", 25) + "next",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderColumns(tt.blocks, tt.maxWidth, tt.columns)
			// Avoiding assert.Equal so that escape sequences are not written raw to stdout.
			if got != tt.want {
				t.Errorf("got:\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

// TestRenderColumnsNoTrailingWhitespace guards the padding of the final cell in
// each row: the columns must line up without leaving invisible whitespace that
// shows up when a user selects or copies the output.
func TestRenderColumnsNoTrailingWhitespace(t *testing.T) {
	blocks := [][]string{{"a1", "a2", "a3"}, {"b1"}, {"c1", "c2"}}

	for _, line := range strings.Split(RenderColumns(blocks, 60, 3), "\n") {
		assert.Equal(t, strings.TrimRight(line, " "), line, "line has trailing whitespace")
	}
}

// TestRenderColumnsTruncationResetsColor ensures a color started in one column
// cannot bleed into the next when its line is cut short.
func TestRenderColumnsTruncationResetsColor(t *testing.T) {
	blocks := [][]string{{"\x1b[32mabcdefghijklmnopqrstuvwxyz0123\x1b[0m"}, {"plain"}}

	got := RenderColumns(blocks, 52, 2)
	assert.True(t, strings.HasPrefix(got, "\x1b[32mabcdefghijklmnopqrstuv...\x1b[0m"), "got: %q", got)
}

// TestRenderColumnsFitsWithinMaxWidth guards the column arithmetic against the
// rounding that integer division invites. A row wider than the terminal would
// wrap, which would cost back the vertical space the columns were meant to save.
//
// Single-column output is exempt: it reproduces the long-standing full-width
// layout, which lets long job names run past the edge rather than truncating
// them, and that behavior should not change for people who never ask for
// columns.
func TestRenderColumnsFitsWithinMaxWidth(t *testing.T) {
	blocks := [][]string{
		{"a job whose name runs well past any sensible column width", "  a step"},
		{"short"},
		{"medium length job name", "  step one", "  step two", "  step three"},
		{"another job", "  step"},
		{"yet another job with a fairly long name", "  step"},
	}

	for maxWidth := 20; maxWidth <= 200; maxWidth++ {
		for columns := 2; columns <= 5; columns++ {
			if fitColumns(len(blocks), maxWidth, columns) == 1 {
				continue
			}
			for _, line := range strings.Split(RenderColumns(blocks, maxWidth, columns), "\n") {
				if got := text.DisplayWidth(line); got > maxWidth {
					t.Fatalf("maxWidth=%d columns=%d: line of width %d exceeds max: %q",
						maxWidth, columns, got, line)
				}
			}
		}
	}
}

func TestRenderColumnsPreservesJobOrderDownColumns(t *testing.T) {
	blocks := [][]string{{"a"}, {"b"}, {"c"}, {"d"}, {"e"}, {"f"}}

	got := RenderColumns(blocks, 60, 2)

	assert.Equal(t, strings.Join([]string{
		"a" + strings.Repeat(" ", 30) + "d",
		"b" + strings.Repeat(" ", 30) + "e",
		"c" + strings.Repeat(" ", 30) + "f",
	}, "\n"), got)
}
