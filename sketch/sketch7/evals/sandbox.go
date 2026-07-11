package evals

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// defaultSandboxExcludes are relative-path prefixes never copied into a
// sandbox. `.git` is excluded unconditionally so nothing inside a sandbox can
// mutate or push repository state; the rest are heavyweight cache/output
// trees that no write microtask should depend on.
var defaultSandboxExcludes = []string{
	".git",
	".lumora",
	".maelstrom",
	".empirica",
	".ruff_cache",
}

// createSandbox copies the source tree into a fresh temporary directory and
// returns the sandbox root plus a cleanup func. Write-enabled eval sessions
// run entirely against the copy: tools are rooted there, so the worst a
// session can do is ruin its own disposable tree. Symlinks are skipped (not
// followed) so a link cannot smuggle writes back outside the sandbox.
func createSandbox(source string, extraExcludes []string) (string, func(), error) {
	absSource, err := filepath.Abs(source)
	if err != nil {
		return "", nil, fmt.Errorf("resolve sandbox source: %w", err)
	}
	info, err := os.Stat(absSource)
	if err != nil {
		return "", nil, fmt.Errorf("stat sandbox source: %w", err)
	}
	if !info.IsDir() {
		return "", nil, fmt.Errorf("sandbox source %q is not a directory", source)
	}
	excludes := append(append([]string{}, defaultSandboxExcludes...), extraExcludes...)
	dir, err := os.MkdirTemp("", "maelstrom-sandbox-")
	if err != nil {
		return "", nil, fmt.Errorf("create sandbox dir: %w", err)
	}
	cleanup := func() { os.RemoveAll(dir) }
	if err := copyTree(absSource, dir, excludes); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("populate sandbox: %w", err)
	}
	return dir, cleanup, nil
}

func copyTree(source, dest string, excludes []string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if excludedPath(rel, excludes) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dest, rel)
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			// Skip symlinks entirely: a link pointing outside the tree would
			// let sandboxed writes land outside the sandbox.
			return nil
		case info.IsDir():
			return os.MkdirAll(target, info.Mode().Perm())
		case info.Mode().IsRegular():
			return copyFile(path, target, info.Mode().Perm())
		default:
			// Sockets, devices, pipes: never copied.
			return nil
		}
	})
}

// excludedPath reports whether rel (a slash-agnostic relative path) matches an
// exclude entry exactly or lives beneath one.
func excludedPath(rel string, excludes []string) bool {
	normalized := filepath.ToSlash(rel)
	for _, exclude := range excludes {
		entry := strings.Trim(filepath.ToSlash(strings.TrimSpace(exclude)), "/")
		if entry == "" {
			continue
		}
		if normalized == entry || strings.HasPrefix(normalized, entry+"/") {
			return true
		}
	}
	return false
}

func copyFile(source, dest string, perm os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
