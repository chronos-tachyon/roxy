package membership

var nullBytes = []byte("null")

type enumData struct {
	GoName string
	Name   string
}

type enumJSON struct {
	index uint
	bytes []byte
}
