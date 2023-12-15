package state

func identity() *matrix {
	return &matrix{
		{1, 0, 0},
		{0, 1, 0},
		{0, 0, 1},
	}
}

type matrix [3][3]float64

func (m *matrix) Mul(n *matrix) *matrix {

	if m.sparse() && n.sparse() {
		return &matrix{
			{m[0][0] * n[0][0], 0, 0},
			{0, m[1][1] * n[1][1], 0},
			{m[2][0]*n[0][0] + n[2][0], m[2][1]*n[1][1] + n[2][1], 1},
		}
	}

	var mn matrix
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			for k := 0; k < 3; k++ {
				mn[i][j] += m[i][k] * n[k][j]
			}
		}
	}

	return &mn
}

func (m *matrix) sparse() bool {
	return m[0][1] == 0 && m[0][2] == 0 &&
		m[1][0] == 0 && m[1][2] == 0 &&
		m[2][2] == 1
}
