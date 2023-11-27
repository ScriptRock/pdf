package encoding

type None struct{}

func (e None) Decode(raw string) string { return raw }
