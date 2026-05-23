package context

type ChunkSpec struct {
	Chunk     ContextChunk
	BudgetPct float64          // fraction of total budget
	Policy    TruncationPolicy // how to trim if over budget
	Priority  int              // higher = more important, trimmed last
	Flexible  bool             // if true, expands to fill remaining budget
}
