package prompt

type ProvenanceStep struct {
	ProjectionName    string
	Operation         string
	InputRecordIDs    []string
	OutputDescriptor  string
	RuntimeDescriptor string
}
