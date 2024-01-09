package types

// A name is a PDF name, without the leading slash.
type Name string

// An object is a PDF syntax object, one of the following Go types:
//
//	bool, a PDF boolean
//	int64, a PDF integer
//	float64, a PDF real
//	string, a PDF string literal
//	name, a PDF name without the leading slash
//	dict, a PDF dictionary
//	array, a PDF array
//	stream, a PDF stream
//	objptr, a PDF object reference
//	objdef, a PDF object definition
//
// An object may also be nil, to represent the PDF null.
type Object any

type Dict map[Name]Object

type Array []Object

type Stream struct {
	Hdr    Dict
	Ptr    Objptr
	Offset int64
}

type Objptr struct {
	ID  uint32
	Gen uint16
}

type Objdef struct {
	Ptr Objptr
	Obj Object
}

type Xref struct {
	Ptr      Objptr
	InStream bool
	Stream   Objptr
	Offset   int64
}
