package registry

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
)

type FileIngestor interface {
	IngestFile(ctx context.Context, path string) error
}

type Watcher struct {
	FSWatcher *fsnotify.Watcher
	Ingestor  FileIngestor
	mu        sync.Mutex
	watched   map[string]struct{}
}

func NewWatcher(ingestor FileIngestor) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Watcher{FSWatcher: fw, Ingestor: ingestor, watched: map[string]struct{}{}}, nil
}

func (w *Watcher) Close() error {
	return w.FSWatcher.Close()
}

func (w *Watcher) WatchPath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return filepath.WalkDir(path, func(current string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return w.add(current)
			}
			return nil
		})
	}
	return w.add(filepath.Dir(path))
}

func (w *Watcher) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-w.FSWatcher.Errors:
			if err != nil {
				return err
			}
		case event := <-w.FSWatcher.Events:
			if event.Name == "" {
				continue
			}
			if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename) == 0 {
				continue
			}
			if err := w.Ingestor.IngestFile(ctx, event.Name); err != nil {
				return fmt.Errorf("watch ingest %s: %w", event.Name, err)
			}
		}
	}
}

func (w *Watcher) add(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.watched[path]; ok {
		return nil
	}
	if err := w.FSWatcher.Add(path); err != nil {
		return err
	}
	w.watched[path] = struct{}{}
	return nil
}
