package prompt

type Segment interface {
	SegmentKind() string
	TokenText() string
	SourceRecordIDs() []string
	ProvenanceStep() ProvenanceStep
}

type PromptSegment struct {
	Role      string
	Name      string
	CallID    string
	Content   string
	RecordIDs []string
	Step      ProvenanceStep
}

func (p PromptSegment) SegmentKind() string            { return "prompt" }
func (p PromptSegment) TokenText() string              { return p.Content }
func (p PromptSegment) SourceRecordIDs() []string      { return append([]string(nil), p.RecordIDs...) }
func (p PromptSegment) ProvenanceStep() ProvenanceStep { return p.Step }

type StateSegment struct {
	Name      string
	State     string
	RecordIDs []string
	Step      ProvenanceStep
}

func (s StateSegment) SegmentKind() string            { return "state" }
func (s StateSegment) TokenText() string              { return s.State }
func (s StateSegment) SourceRecordIDs() []string      { return append([]string(nil), s.RecordIDs...) }
func (s StateSegment) ProvenanceStep() ProvenanceStep { return s.Step }
