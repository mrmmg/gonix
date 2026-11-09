package tui

import (
	"sort"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// Scores of the fuzzy matcher. A contiguous match always beats a scattered
// one (substringScore is larger than any subsequence can reach).
const (
	substringScore = 1000
	charScore      = 16
	adjacentBonus  = 12
	boundaryBonus  = 10
	gapPenalty     = 1
	minFuzzyLen    = 2 // shorter words only match as a substring
)

func lowerRunes(s string) []rune {
	r := []rune(s)
	for i := range r {
		r[i] = unicode.ToLower(r[i])
	}
	return r
}

func isBoundary(text []rune, j int) bool {
	return j == 0 || !(unicode.IsLetter(text[j-1]) || unicode.IsDigit(text[j-1]))
}

// fuzzyMatch finds term in text, ignoring case: first as a plain substring,
// then as scattered letters in order ("shpex" finds "shop.example.com").
// It returns a score (higher is better) and the rune positions matched.
func fuzzyMatch(term, text string) (int, []int, bool) {
	t, x := lowerRunes(term), lowerRunes(text)
	n, m := len(t), len(x)
	if n == 0 || n > m {
		return 0, nil, false
	}
	// A substring, preferring one that starts a word, then the earliest.
	best := -1
	for i := 0; i+n <= m; i++ {
		if string(x[i:i+n]) != string(t) {
			continue
		}
		if best < 0 || (isBoundary(x, i) && !isBoundary(x, best)) {
			best = i
		}
	}
	if best >= 0 {
		pos := make([]int, n)
		for k := range pos {
			pos[k] = best + k
		}
		score := substringScore - best
		if isBoundary(x, best) {
			score += boundaryBonus
		}
		return score, pos, true
	}
	if n < minFuzzyLen {
		return 0, nil, false
	}

	// Scattered letters, best score by dynamic programming: score[i][j] is
	// the best way to match t[:i+1] with t[i] at x[j]. A gap costs 1 per
	// skipped character, which lets the best predecessor be tracked as a
	// running maximum instead of searched.
	const none = -1 << 30
	score := make([][]int, n)
	from := make([][]int, n)
	for i := range score {
		score[i], from[i] = make([]int, m), make([]int, m)
		for j := range score[i] {
			score[i][j], from[i][j] = none, -1
		}
	}
	for i := 0; i < n; i++ {
		runBest, runAt := none, -1 // max of score[i-1][k]+k over k <= j-2
		for j := i; j < m; j++ {
			if i > 0 && j >= 2 && score[i-1][j-2] > none && score[i-1][j-2]+(j-2) > runBest {
				runBest, runAt = score[i-1][j-2]+(j-2), j-2
			}
			if x[j] != t[i] {
				continue
			}
			s := charScore
			if isBoundary(x, j) {
				s += boundaryBonus
			}
			if i == 0 {
				score[i][j] = s - min(j, 3)*gapPenalty
				continue
			}
			bestPrev, at := none, -1
			if j >= 1 && score[i-1][j-1] > none {
				bestPrev, at = score[i-1][j-1]+adjacentBonus, j-1
			}
			if runAt >= 0 && runBest-(j-1) > bestPrev {
				bestPrev, at = runBest-(j-1), runAt
			}
			if at >= 0 {
				score[i][j], from[i][j] = s+bestPrev, at
			}
		}
	}
	bestJ, bestS := -1, none
	for j := 0; j < m; j++ {
		if score[n-1][j] > bestS {
			bestJ, bestS = j, score[n-1][j]
		}
	}
	if bestJ < 0 {
		return 0, nil, false
	}
	pos := make([]int, n)
	for i, j := n-1, bestJ; i >= 0; i-- {
		pos[i] = j
		j = from[i][j]
	}
	return bestS, pos, true
}

// itemMatch is how well a menu row matches a query, and which characters of
// its title and description to highlight.
type itemMatch struct {
	score       int
	title, desc map[int]bool
}

// matchItem matches every word of query against the row's title,
// description or extra search text; all words must match somewhere.
func matchItem(query string, it menuItem) (itemMatch, bool) {
	title, desc := ansi.Strip(it.title), ansi.Strip(it.desc)
	m := itemMatch{title: map[int]bool{}, desc: map[int]bool{}}
	for _, word := range strings.Fields(query) {
		bestScore, where := 0, -1
		var bestPos []int
		for field, text := range []string{title, desc, it.search} {
			s, pos, ok := fuzzyMatch(word, text)
			if !ok {
				continue
			}
			s *= 3 - field // the title counts most
			if s > bestScore {
				bestScore, where, bestPos = s, field, pos
			}
		}
		if where < 0 {
			return itemMatch{}, false
		}
		m.score += bestScore
		for _, p := range bestPos {
			switch where {
			case 0:
				m.title[p] = true
			case 1:
				m.desc[p] = true
			}
		}
	}
	return m, true
}

// rank sorts matches best first, keeping the original order of equals.
func rank(idx []int, ms []itemMatch) {
	sort.SliceStable(idx, func(a, b int) bool { return ms[idx[a]].score > ms[idx[b]].score })
}

// highlight marks the visible characters of s at the given positions
// (rune offsets into its text without escape sequences) with an underline
// and reversed colours, which works in any terminal theme and leaves the
// colours already in s alone.
func highlight(s string, pos map[int]bool) string {
	if len(pos) == 0 {
		return s
	}
	const on, off = "\x1b[4;7m", "\x1b[24;27m"
	var b strings.Builder
	visible, marked := 0, false
	r := []rune(s)
	for i := 0; i < len(r); i++ {
		if r[i] == 0x1b && i+1 < len(r) && r[i+1] == '[' {
			j := i + 2
			for j < len(r) && !(r[j] >= 0x40 && r[j] <= 0x7e) {
				j++
			}
			if marked { // keep the escape sequence outside the marked run
				b.WriteString(off)
				marked = false
			}
			b.WriteString(string(r[i:min(j+1, len(r))]))
			i = j
			continue
		}
		hit := pos[visible]
		if hit && !marked {
			b.WriteString(on)
			marked = true
		} else if !hit && marked {
			b.WriteString(off)
			marked = false
		}
		b.WriteRune(r[i])
		visible++
	}
	if marked {
		b.WriteString(off)
	}
	return b.String()
}
