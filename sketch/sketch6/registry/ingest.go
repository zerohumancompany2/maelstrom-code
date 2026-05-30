package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

type Ingestor[D any, S any] struct {
	Decoder  Decoder[D]
	Hoister  Hoister[D, S]
	Store    Store[S]
	Registry Registry[S]
	Hooks    []Preprocessor
	NewID    func(key string) string
}

func (i Ingestor[D, S]) Ingest(ctx context.Context, key, path string, raw []byte) (IngestResult[S], error) {
	sourceHash := hashBytes(raw)
	processed := append([]byte(nil), raw...)
	var err error
	for _, hook := range i.Hooks {
		processed, err = hook.Apply(processed)
		if err != nil {
			return IngestResult[S]{}, fmt.Errorf("preprocess %s: %w", hook.Name(), err)
		}
	}

	processedHash := hashBytes(processed)
	latest, ok, err := i.Store.GetLatest(ctx, key)
	if err != nil {
		return IngestResult[S]{}, err
	}
	if ok && latest.ProcessedHash == processedHash {
		return IngestResult[S]{Revision: latest, Changed: false}, nil
	}

	doc, err := i.Decoder.Decode(processed)
	if err != nil {
		return IngestResult[S]{}, err
	}
	value, err := i.Hoister.Hoist(doc)
	if err != nil {
		return IngestResult[S]{}, err
	}

	revision := Revision[S]{
		ID:            i.NewID(key),
		Key:           key,
		SourcePath:    path,
		SourceHash:    sourceHash,
		ProcessedHash: processedHash,
		Value:         value,
	}
	if err := i.Store.Put(ctx, revision); err != nil {
		return IngestResult[S]{}, err
	}
	if err := i.Registry.Update(revision); err != nil {
		return IngestResult[S]{}, err
	}
	return IngestResult[S]{Revision: revision, Changed: true}, nil
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
