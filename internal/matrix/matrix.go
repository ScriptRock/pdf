package matrix

func Identity() *Matrix {
	return &Matrix{
		{1, 0, 0},
		{0, 1, 0},
		{0, 0, 1},
	}
}

type Matrix [3][3]float64

func (m *Matrix) Mul(n *Matrix) *Matrix {
	var mn Matrix

	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			for k := 0; k < 3; k++ {
				mn[i][j] += m[i][k] * n[k][j]
			}
		}
	}

	return &mn
}
