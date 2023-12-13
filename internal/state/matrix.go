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
