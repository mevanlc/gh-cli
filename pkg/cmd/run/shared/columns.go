package shared

import (
	"strings"

	"github.com/cli/cli/v2/internal/text"
)

const (
	// AutoColumnCount is the sentinel column count meaning "fit as many columns
	// as the terminal is wide enough for", as requested by `--columns auto`.
	AutoColumnCount = -1

	// columnGutter is how many blank cells separate adjacent columns.
	columnGutter = 2

	// minColumnWidth is the narrowest a column may become. Below this, job names
	// truncate down to little more than an ellipsis, so an explicitly requested
	// column count is reduced rather than honored.
	minColumnWidth = 24

	// preferredColumnWidth is the width AutoColumns aims to give each column. It
	// comfortably fits a status symbol, a matrix job name such as
	// "build (ubuntu-latest, 1.25)", and its elapsed time.
	preferredColumnWidth = 40

	// maxAutoColumns bounds AutoColumns so that a very wide terminal does not
	// scatter a short job list across so many columns that it stops being
	// scannable. An explicit `--columns N` is not subject to this.
	maxAutoColumns = 4
)

// AutoColumns reports how many columns of job output fit within maxWidth.
func AutoColumns(maxWidth int) int {
	columns := (maxWidth + columnGutter) / (preferredColumnWidth + columnGutter)
	if columns > maxAutoColumns {
		return maxAutoColumns
	}
	if columns < 1 {
		return 1
	}

	return columns
}

// RenderColumns lays blocks out in up to the requested number of columns,
// filling each column top to bottom before starting the next so that jobs still
// read in their original order. A block is never split across a column
// boundary, and lines wider than a column are truncated rather than wrapped, so
// that every job stays on the row it started on.
//
// The requested count is reduced when there are fewer blocks than columns, or
// when maxWidth cannot give each column at least minColumnWidth. A single
// column reproduces the plain full-width layout byte for byte, which lets
// callers route every rendering path through here.
func RenderColumns(blocks [][]string, maxWidth, columns int) string {
	columns = fitColumns(len(blocks), maxWidth, columns)
	if columns == 1 {
		return joinBlocks(blocks)
	}

	columnWidth := (maxWidth - columnGutter*(columns-1)) / columns
	packed := packBlocks(blocks, columns)

	height := 0
	for _, column := range packed {
		if len(column) > height {
			height = len(column)
		}
	}

	gutter := strings.Repeat(" ", columnGutter)
	rows := make([]string, 0, height)

	for row := 0; row < height; row++ {
		cells := make([]string, len(packed))
		// Trailing empty cells are dropped instead of padded so that short rows
		// do not leave invisible whitespace at the end of every line.
		last := -1
		for i, column := range packed {
			if row >= len(column) {
				continue
			}
			cells[i] = text.Truncate(columnWidth, column[row])
			if cells[i] != "" {
				last = i
			}
		}

		parts := make([]string, 0, last+1)
		for i := 0; i <= last; i++ {
			if i == last {
				parts = append(parts, cells[i])
			} else {
				parts = append(parts, text.PadRight(columnWidth, cells[i]))
			}
		}

		rows = append(rows, strings.Join(parts, gutter))
	}

	return strings.Join(rows, "\n")
}

// fitColumns reduces a requested column count to one the content and the
// terminal can actually support.
func fitColumns(blockCount, maxWidth, columns int) int {
	if columns > blockCount {
		columns = blockCount
	}
	for columns > 1 && (maxWidth-columnGutter*(columns-1))/columns < minColumnWidth {
		columns--
	}
	if columns < 1 {
		return 1
	}

	return columns
}

// packBlocks distributes blocks over at most n columns by searching for the
// shortest column height that still fits within n columns. Balancing on height
// rather than dealing out a fixed number of blocks per column matters because
// jobs vary a lot in step count: a run with one 30-step job and six 2-step jobs
// would otherwise leave one column enormous and the rest nearly empty.
func packBlocks(blocks [][]string, n int) [][]string {
	// The shortest conceivable column holds the tallest single block, since
	// blocks are never split; the tallest holds everything, which always fits in
	// one column. So the answer is somewhere in [lo, hi] and hi is always valid.
	lo, hi := 0, 0
	for _, block := range blocks {
		if len(block) > lo {
			lo = len(block)
		}
		hi += len(block)
	}

	height := hi
	for lo <= hi {
		mid := (lo + hi) / 2
		if columnsNeeded(blocks, mid) <= n {
			height = mid
			hi = mid - 1
		} else {
			lo = mid + 1
		}
	}

	columns := make([][]string, 0, n)
	var current []string
	for _, block := range blocks {
		// The column count guard keeps everything that is left in the final
		// column, so the result can never exceed n columns.
		if len(current) > 0 && len(current)+len(block) > height && len(columns) < n-1 {
			columns = append(columns, current)
			current = nil
		}
		current = append(current, block...)
	}

	return append(columns, current)
}

// columnsNeeded counts how many columns a greedy top-to-bottom fill uses when no
// column may exceed height lines.
func columnsNeeded(blocks [][]string, height int) int {
	used, current := 1, 0
	for _, block := range blocks {
		if current > 0 && current+len(block) > height {
			used++
			current = 0
		}
		current += len(block)
	}

	return used
}
