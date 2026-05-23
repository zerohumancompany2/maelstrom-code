package context

import "fmt"

type TruncationPolicy interface {
	Trim(items []ContextItem, budget int, tokenizer Tokenizer) ([]ContextItem, error)
}

type HardTruncate struct{}

type FailTruncate struct{}

func (ht *HardTruncate) Trim(items []ContextItem, budget int, tokenizer Tokenizer) ([]ContextItem, error) {
	if budget <= 0 || len(items) == 0 {
		return []ContextItem{}, nil
	}

	count := 0
	reversed := make([]ContextItem, 0, len(items))

	for i := len(items) - 1; i >= 0; i-- {
		itemTokens := tokenizer.CountTokens(items[i])
		if count+itemTokens > budget {
			break
		}
		count += itemTokens
		reversed = append(reversed, items[i])
	}

	out := make([]ContextItem, len(reversed))
	for i := range reversed {
		out[len(reversed)-1-i] = reversed[i]
	}
	return out, nil
}

func (ft *FailTruncate) Trim(items []ContextItem, budget int, tokenizer Tokenizer) ([]ContextItem, error) {
	if tokenizer.CountTokens(items...) > budget {
		return nil, fmt.Errorf("context exceeds non-truncatable budget")
	}
	return items, nil
}
