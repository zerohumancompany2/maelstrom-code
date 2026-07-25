package tools

import "testing"

func TestSearchFilesRanksMdOutsideDocsDir(t *testing.T) {
	matches := []SearchMatch{
		{Path: "tools/replace_text.md", Line: 5, Snippet: "replace_text tool documentation"},
		{Path: "tools/replace_text_test.go", Line: 20, Snippet: "func TestReplaceText(t *testing.T) {}"},
		{Path: "tools/replace_text.go", Line: 14, Snippet: "func (ReplaceTextTool) Definition() Definition {"},
	}
	ranked := rankSearchMatches("replace_text", matches, 10)
	if ranked[0].Path != "tools/replace_text.go" {
		t.Fatalf("top = %q, want implementation first", ranked[0].Path)
	}
	// .md should be last, even outside /docs/
	if ranked[2].Path != "tools/replace_text.md" {
		t.Fatalf("last = %q, want .md file last", ranked[2].Path)
	}
}
