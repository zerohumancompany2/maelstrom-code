package prompt

import "fmt"

type Assembler struct {
	Projections []Projection
}

type Result struct {
	Segments []Segment
	Steps    []ProvenanceStep
}

func (a Assembler) Assemble(input Input) (Result, error) {
	result := Result{}
	for _, projection := range a.Projections {
		projectionResult, err := projection.Build(input)
		if err != nil {
			return Result{}, fmt.Errorf("build projection %s: %w", projection.Name(), err)
		}
		result.Segments = append(result.Segments, projectionResult.Segments...)
		result.Steps = append(result.Steps, projectionResult.Steps...)
	}
	return result, nil
}
