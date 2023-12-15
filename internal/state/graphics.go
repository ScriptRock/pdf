package state

// Graphics holds some state defined in:
// PDF_ISO_32000-2: Table 51: Device-independent graphics state parameters
// and
// PDF_ISO_32000-2: 8.4.2 Graphics state stack
//
// Methods on Graphics implement some operators from:
// PDF_ISO_32000-2: Table 56: Graphics state operators
type Graphics struct {
	gState
	stack []gState
}

type gState struct {
	ctm *matrix
	Text
}

func (g *Graphics) Push() {
	if g.gState.ctm == nil {
		g.gState.ctm = identity()
	}

	g.stack = append(g.stack, g.gState)
}

func (g *Graphics) Pop() {
	n := len(g.stack)
	g.gState = g.stack[n-1]
	g.stack = g.stack[:n-1]
}

func (g *Graphics) Tj(r Renderer, raw string) {
	if g.gState.ctm == nil {
		g.gState.ctm = identity()
	}
	g.gState.Text.Tj(g.gState.ctm, r, raw)
}

func (g *Graphics) CM(a, b, c, d, e, f float64) {
	m := &matrix{
		{a, b, 0},
		{c, d, 0},
		{e, f, 1},
	}
	if g.gState.ctm == nil {
		g.gState.ctm = m
	} else {
		g.gState.ctm = m.Mul(g.gState.ctm)
	}
}
