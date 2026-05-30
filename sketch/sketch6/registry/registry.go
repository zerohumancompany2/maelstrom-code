package registry

import "context"

type Revision[T any] struct {
	ID            string
	Key           string
	SourcePath    string
	SourceHash    string
	ProcessedHash string
	Value         T
}

type Store[T any] interface {
	Put(ctx context.Context, revision Revision[T]) error
	GetLatest(ctx context.Context, key string) (Revision[T], bool, error)
}

type Registry[T any] interface {
	Update(revision Revision[T]) error
	Get(key string) (Revision[T], bool)
	List() []Revision[T]
}

type Decoder[D any] interface {
	Decode(data []byte) (D, error)
}

type Hoister[D any, S any] interface {
	Hoist(doc D) (S, error)
}

type Preprocessor interface {
	Name() string
	Apply(input []byte) ([]byte, error)
}

type IngestResult[T any] struct {
	Revision Revision[T]
	Changed  bool
}

type MemoryRegistry[T any] struct {
	byKey map[string]Revision[T]
}

func NewMemoryRegistry[T any]() *MemoryRegistry[T] {
	return &MemoryRegistry[T]{byKey: map[string]Revision[T]{}}
}

func (r *MemoryRegistry[T]) Update(revision Revision[T]) error {
	r.byKey[revision.Key] = revision
	return nil
}

func (r *MemoryRegistry[T]) Get(key string) (Revision[T], bool) {
	revision, ok := r.byKey[key]
	return revision, ok
}

func (r *MemoryRegistry[T]) List() []Revision[T] {
	items := make([]Revision[T], 0, len(r.byKey))
	for _, revision := range r.byKey {
		items = append(items, revision)
	}
	return items
}

type MemoryStore[T any] struct {
	latest map[string]Revision[T]
}

func NewMemoryStore[T any]() *MemoryStore[T] {
	return &MemoryStore[T]{latest: map[string]Revision[T]{}}
}

func (s *MemoryStore[T]) Put(_ context.Context, revision Revision[T]) error {
	s.latest[revision.Key] = revision
	return nil
}

func (s *MemoryStore[T]) GetLatest(_ context.Context, key string) (Revision[T], bool, error) {
	revision, ok := s.latest[key]
	return revision, ok, nil
}
