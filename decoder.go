package pdf

// A Decoder represents a mapping between
// font code points and UTF-8 text.
type Decoder interface {
	// Decode returns the UTF-8 text corresponding to
	// the sequence of code points in raw.
	Decode(raw string) (string, float64)
}
