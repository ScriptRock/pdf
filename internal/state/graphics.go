package state

import "github.com/njupg/pdf/internal/matrix"

// Graphics holds some state defined in:
// PDF_ISO_32000-2: Table 51: Device-independent graphics state parameters
// and
// PDF_ISO_32000-2: 8.4.2 Graphics state stack
//
// Methods on Graphics implement some operators from:
// PDF_ISO_32000-2: Table 56: Graphics state operators
type Graphics struct {
	ctm   *matrix.Matrix
	stack []*matrix.Matrix
}

func (g *Graphics) Push() {
	if g.ctm == nil {
		g.ctm = matrix.Identity()
	}

	g.stack = append(g.stack, g.ctm)
}

func (g *Graphics) Pop() {
	n := len(g.stack)
	g.ctm = g.stack[n-1]
	g.stack = g.stack[:n-1]
}

func (g *Graphics) CM(a, b, c, d, e, f float64) {
	m := &matrix.Matrix{
		{a, b, 0},
		{c, d, 0},
		{e, f, 1},
	}
	if g.ctm == nil {
		g.ctm = m
	} else {
		g.ctm = m.Mul(g.ctm)
	}
}
