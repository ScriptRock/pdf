package encoding

import "strings"

type Dict struct {
	Elements []any
}

func (e *Dict) Decode(raw string) string {
	var r strings.Builder
	for i := 0; i < len(raw); i++ {
		ch := rune(raw[i])
		n := -1
		for _, x := range e.Elements {
			switch v := x.(type) {
			case int64:
				n = int(v)
				continue
			case string:
				if int(raw[i]) == n {
					r := nameToRune[v]
					if r != 0 {
						ch = r
						break
					}
				}
				n++
			}
		}
		r.WriteRune(ch)
	}
	return r.String()
}
