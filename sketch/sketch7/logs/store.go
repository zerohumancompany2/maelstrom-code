package logs

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type SessionStore interface {
	LoadSession(sessionID string) (*SessionHistory, *WorkflowHistory, error)
	ListSessions() ([]string, error)
	SaveSession(sessionID string, session *SessionHistory, workflow *WorkflowHistory) error
}

type FileSessionStore struct {
	SessionDir string
}

func (s FileSessionStore) LoadSession(sessionID string) (*SessionHistory, *WorkflowHistory, error) {
	path := filepath.Join(s.SessionDir, sessionID+".json")
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, os.ErrNotExist
		}
		return nil, nil, err
	}
	return LoadState(path)
}

func (s FileSessionStore) ListSessions() ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(s.SessionDir, "*.json"))
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		base := filepath.Base(match)
		ids = append(ids, strings.TrimSuffix(base, filepath.Ext(base)))
	}
	sort.Strings(ids)
	return ids, nil
}

func (s FileSessionStore) SaveSession(sessionID string, session *SessionHistory, workflow *WorkflowHistory) error {
	return SaveState(filepath.Join(s.SessionDir, sessionID+".json"), session, workflow)
}
