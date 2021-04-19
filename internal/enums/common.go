package enums

type enumData struct {
	GoName string
	Name   string
}

func makeAllowedNames(data []enumData) []string {
	out := make([]string, len(data))
	for index, row := range data {
		out[index] = row.Name
	}
	return out
}
