package state

// identity returns the identity matrix.
func identity() *matrix {
	return &matrix{
		{1, 0, 0},
		{0, 1, 0},
		{0, 0, 1},
	}
}

// matrix represents 3x3 matrices for scale, rotation and translation for text and graphics state.
// The final column of these matrixes is always 0,0,1.
type matrix [3][3]float64

func (m *matrix) Mul(n *matrix) *matrix {

	if !m.rotation() && !n.rotation() {
		return &matrix{
			{m[0][0] * n[0][0], 0, 0},
			{0, m[1][1] * n[1][1], 0},
			{m[2][0]*n[0][0] + n[2][0], m[2][1]*n[1][1] + n[2][1], 1},
		}
	}

	return &matrix{
		{m[0][0]*n[0][0] + m[0][1]*n[1][0], m[0][0]*n[0][1] + m[0][1]*n[1][1], 0},
		{m[1][0]*n[0][0] + m[1][1]*n[1][0], m[1][0]*n[0][1] + m[1][1]*n[1][1], 0},
		{m[2][0]*n[0][0] + m[2][1]*n[1][0] + n[2][0], m[2][0]*n[0][1] + m[2][1]*n[1][1] + n[2][1], 1},
	}
}

func (m *matrix) rotation() bool {
	return m[0][1] != 0 || m[1][0] != 0
}
