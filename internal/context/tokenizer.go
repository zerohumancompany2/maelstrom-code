package context

import "math"

type Tokenizer interface {
	CountTokens(items ...ContextItem) int
}

type HeuristicTokenizer struct{}

func (ht *HeuristicTokenizer) CountTokens(items ...ContextItem) int {
	total := 0
	for _, item := range items {
		total += countItemTokens(item)
	}
	return total
}

func countItemTokens(item ContextItem) int {
	content := item.TokenText()
	if content == "" {
		return 0
	}
	return int(math.Ceil(float64(len(content)) / 4.0))
}
