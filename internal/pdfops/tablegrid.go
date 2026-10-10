package pdfops

import (
	"math"
	"sort"
)

// Ruled tables (ADR-121): a table is read from the rules the page draws.
//
// The proposer knows a table by one thing only — a REGULAR ruled grid: horizontal and vertical rules that
// bound at least two rows and two columns of cells, every cell's four sides drawn. Text with no rules is never
// a table here, however it is aligned: columns of figures, a two-column page and a price list all line up, and
// a rule that guessed from alignment would call each of them one.
//
// This file is geometry over the page map's pieces (`pageShape`, ADR-088) and names no run. Which run sits in
// which cell is the grouping door's (`groupRunsAndTables`).

const (
	// gridTol is how far apart, in points, two coordinates may be and still be one rule: where two rules
	// meet, and where a rule drawn as a thin rectangle has its middle against one drawn as a line.
	gridTol = 1.5
	// gridSlack is how far a rule may run past the grid's outermost rule the other way. Rules drawn as thin
	// rectangles overlap at the corners by their own thickness; a rule that runs further bounds something
	// the lattice does not hold — a column with no side — and the grid is not read.
	gridSlack = 3.0
	// gridMaxCrossings bounds the rule-against-rule comparisons made for one page. A page of vector art draws
	// thousands of straight pieces; no table needs them, and the page is proposed as it was before tables.
	gridMaxCrossings = 250000
)

// ruledGrid is a regular ruled grid in user space.
type ruledGrid struct {
	xs []float64 // the column boundaries, left to right: columns+1 of them
	ys []float64 // the row boundaries, top to bottom: rows+1 of them
}

func (g ruledGrid) rows() int { return len(g.ys) - 1 }
func (g ruledGrid) cols() int { return len(g.xs) - 1 }

// frame is the grid's outer box [llx lly urx ury].
func (g ruledGrid) frame() [4]float64 {
	return [4]float64{g.xs[0], g.ys[len(g.ys)-1], g.xs[len(g.xs)-1], g.ys[0]}
}

// cellBox is one cell's box [llx lly urx ury].
func (g ruledGrid) cellBox(row, col int) [4]float64 {
	return [4]float64{g.xs[col], g.ys[row+1], g.xs[col+1], g.ys[row]}
}

// rowBox is one row's box.
func (g ruledGrid) rowBox(row int) [4]float64 {
	return [4]float64{g.xs[0], g.ys[row+1], g.xs[len(g.xs)-1], g.ys[row]}
}

// cellAt is the cell holding the point (x, y). A point on a boundary belongs to the cell after it.
func (g ruledGrid) cellAt(x, y float64) (row, col int, ok bool) {
	if !inBox(g.frame(), x, y) {
		return 0, 0, false
	}
	for col = 0; col < g.cols()-1 && x >= g.xs[col+1]; col++ {
	}
	for row = 0; row < g.rows()-1 && y <= g.ys[row+1]; row++ {
	}
	return row, col, true
}

func inBox(b [4]float64, x, y float64) bool {
	return x >= b[0] && x <= b[2] && y >= b[1] && y <= b[3]
}

func boxesOverlap(a, b [4]float64) bool {
	return a[0] < b[2] && b[0] < a[2] && a[1] < b[3] && b[1] < a[3]
}

// rule is one straight ruled line: at is its fixed coordinate (y for a horizontal rule, x for a vertical one)
// and from..to its extent along the other.
type rule struct{ at, from, to float64 }

// rulesOf reads the pieces a page paints as horizontal and vertical rules. A straight stroke is a rule; a thin
// rectangle is the rule a producer draws as a filled sliver (`ruleMax`); a stroked box is its four sides, so a
// grid drawn as tiled rectangles reads as the rules those share. A box that is only filled is a ground, not a
// rule, and a piece painted white is not there.
func rulesOf(shapes []pageShape) (h, v []rule) {
	for _, s := range shapes {
		if s.white {
			continue
		}
		b := s.box
		w, ht := b[2]-b[0], b[3]-b[1]
		switch {
		case !s.rect:
			if ht == 0 && w > 0 {
				h = append(h, rule{b[1], b[0], b[2]})
			} else if w == 0 && ht > 0 {
				v = append(v, rule{b[0], b[1], b[3]})
			}
		case ht <= ruleMax && w > ruleMax:
			h = append(h, rule{(b[1] + b[3]) / 2, b[0], b[2]})
		case w <= ruleMax && ht > ruleMax:
			v = append(v, rule{(b[0] + b[2]) / 2, b[1], b[3]})
		case w > ruleMax && ht > ruleMax && s.stroked:
			h = append(h, rule{b[1], b[0], b[2]}, rule{b[3], b[0], b[2]})
			v = append(v, rule{b[0], b[1], b[3]}, rule{b[2], b[1], b[3]})
		}
	}
	return mergeRules(h), mergeRules(v)
}

// mergeRules joins rules that are one line: those within gridTol of one another across, taken to the middle of
// their spread, and along it every piece that overlaps or abuts the last. The result is ordered, so the same
// page always reads as the same rules.
func mergeRules(rs []rule) []rule {
	sort.Slice(rs, func(i, j int) bool {
		if rs[i].at != rs[j].at {
			return rs[i].at < rs[j].at
		}
		if rs[i].from != rs[j].from {
			return rs[i].from < rs[j].from
		}
		return rs[i].to < rs[j].to
	})
	var out []rule
	for i := 0; i < len(rs); {
		j := i + 1
		for j < len(rs) && rs[j].at-rs[j-1].at <= gridTol {
			j++
		}
		line := append([]rule{}, rs[i:j]...)
		at := (rs[i].at + rs[j-1].at) / 2
		sort.Slice(line, func(a, b int) bool {
			if line[a].from != line[b].from {
				return line[a].from < line[b].from
			}
			return line[a].to < line[b].to
		})
		cur := rule{at, line[0].from, line[0].to}
		for _, r := range line[1:] {
			if r.from <= cur.to+gridTol {
				cur.to = math.Max(cur.to, r.to)
				continue
			}
			out = append(out, cur)
			cur = rule{at, r.from, r.to}
		}
		out = append(out, cur)
		i = j
	}
	return out
}

// crosses says whether a horizontal and a vertical rule meet.
func crosses(h, v rule) bool {
	return v.at >= h.from-gridTol && v.at <= h.to+gridTol && h.at >= v.from-gridTol && h.at <= v.to+gridTol
}

// ruledGrids finds the regular grids a page rules, top to bottom then left to right, and the frames of the
// grids it rules that are NOT regular — a complete outer frame of at least two rows and two columns with a
// side missing inside it, which is how a merged cell is drawn. Those are reported, not read (ADR-121).
func ruledGrids(shapes []pageShape) (grids []ruledGrid, merged [][4]float64) {
	h, v := rulesOf(shapes)
	if len(h) < 3 || len(v) < 3 || len(h)*len(v) > gridMaxCrossings {
		return nil, nil
	}
	// Rules that meet are one drawing: union them, horizontal rules first.
	set := make([]int, len(h)+len(v))
	for i := range set {
		set[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		if set[i] != i {
			set[i] = find(set[i])
		}
		return set[i]
	}
	for i, hr := range h {
		for j, vr := range v {
			if crosses(hr, vr) {
				set[find(len(h)+j)] = find(i)
			}
		}
	}
	type drawing struct{ h, v []rule }
	drawings := map[int]*drawing{}
	var order []int
	for i := range set {
		root := find(i)
		d := drawings[root]
		if d == nil {
			d = &drawing{}
			drawings[root] = d
			order = append(order, root)
		}
		if i < len(h) {
			d.h = append(d.h, h[i])
		} else {
			d.v = append(d.v, v[i-len(h)])
		}
	}
	for _, root := range order {
		d := drawings[root]
		g := ruledGrid{xs: distinctAt(d.v), ys: distinctAt(d.h)}
		sort.Sort(sort.Reverse(sort.Float64Slice(g.ys)))
		if g.rows() < 2 || g.cols() < 2 {
			continue
		}
		f := g.frame()
		if !rulesWithin(d.h, f[0], f[2]) || !rulesWithin(d.v, f[1], f[3]) {
			continue
		}
		regular, framed := true, true
		for r := 0; r <= g.rows(); r++ {
			for c := 0; c < g.cols(); c++ {
				if !ruled(d.h, g.ys[r], g.xs[c], g.xs[c+1]) {
					regular = false
					framed = framed && r != 0 && r != g.rows()
				}
			}
		}
		for c := 0; c <= g.cols(); c++ {
			for r := 0; r < g.rows(); r++ {
				if !ruled(d.v, g.xs[c], g.ys[r+1], g.ys[r]) {
					regular = false
					framed = framed && c != 0 && c != g.cols()
				}
			}
		}
		switch {
		case regular:
			grids = append(grids, g)
		case framed:
			merged = append(merged, f)
		}
	}
	sort.SliceStable(grids, func(i, j int) bool {
		a, b := grids[i].frame(), grids[j].frame()
		if a[3] != b[3] {
			return a[3] > b[3]
		}
		return a[0] < b[0]
	})
	return grids, merged
}

// distinctAt is the distinct fixed coordinates of rs, ascending. Rules of one line already share one
// (`mergeRules`).
func distinctAt(rs []rule) []float64 {
	var out []float64
	for _, r := range rs {
		out = append(out, r.at)
	}
	sort.Float64s(out)
	n := 0
	for i, x := range out {
		if i == 0 || x != out[n-1] {
			out[n] = x
			n++
		}
	}
	return out[:n]
}

// rulesWithin says whether every rule's extent stays between lo and hi, give or take gridSlack.
func rulesWithin(rs []rule, lo, hi float64) bool {
	for _, r := range rs {
		if r.from < lo-gridSlack || r.to > hi+gridSlack {
			return false
		}
	}
	return true
}

// ruled says whether a rule at `at` covers from..to.
func ruled(rs []rule, at, from, to float64) bool {
	for _, r := range rs {
		if r.at == at && r.from <= from+gridTol && r.to >= to-gridTol {
			return true
		}
	}
	return false
}
