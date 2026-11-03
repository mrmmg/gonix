// Package diff renders unified diffs of small text files, used to preview
// configuration changes before they are applied (--dry-run).
package diff

import (
	"fmt"
	"strings"
)

// maxCells bounds the line-matching table; larger inputs are shown as a
// full replacement instead.
const maxCells = 4_000_000

// Unified returns a unified diff from a to b with 3 lines of context, or ""
// when they are equal.
func Unified(a, b, nameA, nameB string) string {
	if a == b {
		return ""
	}
	x, y := lines(a), lines(b)
	ops := diffLines(x, y)

	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n+++ %s\n", nameA, nameB)
	const context = 3
	for i := 0; i < len(ops); {
		if ops[i].kind == ' ' {
			i++
			continue
		}
		// Grow a hunk around this change, merging changes that are less
		// than 2*context lines apart.
		start := max(i-context, 0)
		end := i
		for end < len(ops) {
			if ops[end].kind != ' ' {
				end++
				continue
			}
			run := end
			for run < len(ops) && ops[run].kind == ' ' {
				run++
			}
			if run == len(ops) || run-end > 2*context {
				end = min(end+context, len(ops))
				break
			}
			end = run
		}
		aStart, bStart, aLen, bLen := 0, 0, 0, 0
		for k := 0; k < start; k++ {
			if ops[k].kind != '+' {
				aStart++
			}
			if ops[k].kind != '-' {
				bStart++
			}
		}
		for k := start; k < end; k++ {
			if ops[k].kind != '+' {
				aLen++
			}
			if ops[k].kind != '-' {
				bLen++
			}
		}
		fmt.Fprintf(&out, "@@ -%s +%s @@\n", rangeOf(aStart, aLen), rangeOf(bStart, bLen))
		for k := start; k < end; k++ {
			out.WriteString(string(ops[k].kind) + ops[k].text + "\n")
		}
		i = end
	}
	return out.String()
}

func rangeOf(start, n int) string {
	if n == 0 {
		return fmt.Sprintf("%d,0", start)
	}
	if n == 1 {
		return fmt.Sprintf("%d", start+1)
	}
	return fmt.Sprintf("%d,%d", start+1, n)
}

func lines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

type op struct {
	kind byte // ' ', '-', '+'
	text string
}

// diffLines aligns x and y on a longest common subsequence.
func diffLines(x, y []string) []op {
	n, m := len(x), len(y)
	if n*m > maxCells {
		var ops []op
		for _, l := range x {
			ops = append(ops, op{'-', l})
		}
		for _, l := range y {
			ops = append(ops, op{'+', l})
		}
		return ops
	}
	lcs := make([][]int32, n+1)
	for i := range lcs {
		lcs[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if x[i] == y[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var ops []op
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case x[i] == y[j]:
			ops = append(ops, op{' ', x[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, op{'-', x[i]})
			i++
		default:
			ops = append(ops, op{'+', y[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, op{'-', x[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, op{'+', y[j]})
	}
	return ops
}
