package encoding

type Sizer interface {
	CodeWidth(code int) float64
}
