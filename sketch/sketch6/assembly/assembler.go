package assembly

import "fmt"

type Assembler struct {
	Chunks []Chunk
}

func (a Assembler) Assemble(input Input) (Result, error) {
	result := Result{}
	for _, chunk := range a.Chunks {
		chunkResult, err := chunk.Build(input)
		if err != nil {
			return Result{}, fmt.Errorf("build chunk %s: %w", chunk.Name(), err)
		}
		result.Segments = append(result.Segments, chunkResult.Segments...)
		result.Steps = append(result.Steps, chunkResult.Steps...)
	}
	return result, nil
}
