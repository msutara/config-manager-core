package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTheme_Empty(t *testing.T) {
	th := resolveTheme("")
	if th != nil {
		t.Errorf("expected nil for empty name, got %+v", th)
	}
}

func TestResolveTheme_BuiltinDefault(t *testing.T) {
	th := resolveTheme("default")
	if th == nil {
		t.Fatal("expected non-nil theme for built-in 'default'")
	}
}

func TestResolveTheme_BuiltinSolarized(t *testing.T) {
	th := resolveTheme("solarized-dark")
	if th == nil {
		t.Fatal("expected non-nil theme for built-in 'solarized-dark'")
	}
}

func TestResolveTheme_UnknownBuiltin(t *testing.T) {
	// Not a built-in, not a valid file path (relative) → nil.
	th := resolveTheme("nonexistent-theme-name")
	if th != nil {
		t.Errorf("expected nil for unknown non-absolute name, got %+v", th)
	}
}

func TestResolveTheme_FileMissing(t *testing.T) {
	th := resolveTheme(filepath.Join(t.TempDir(), "missing.yaml"))
	if th != nil {
		t.Errorf("expected nil for missing file, got %+v", th)
	}
}

func TestResolveTheme_FileInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad-theme.yaml")
	if err := os.WriteFile(path, []byte("{{invalid yaml"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	th := resolveTheme(path)
	if th != nil {
		t.Errorf("expected nil for invalid YAML, got %+v", th)
	}
}

func TestResolveTheme_FileValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.yaml")
	yaml := `
colors:
  header_fg: "#ff0000"
glyphs:
  separator_width: 3
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	th := resolveTheme(path)
	if th == nil {
		t.Fatal("expected non-nil theme for valid YAML file")
	}
	if th.SepWidth != 3 {
		t.Errorf("SepWidth: got %d, want 3", th.SepWidth)
	}
}

func TestResolveTheme_RelativePath(t *testing.T) {
	// Relative paths are rejected (must be absolute).
	th := resolveTheme("relative/path/theme.yaml")
	if th != nil {
		t.Errorf("expected nil for relative path, got %+v", th)
	}
}

func TestResolveTheme_PathTraversal(t *testing.T) {
	// Build a platform-absolute traversal path that resolves to a real file,
	// proving rejection comes from the ".." check, not a downstream error.
	dir := t.TempDir()
	legit := filepath.Join(dir, "legit.yaml")
	if err := os.WriteFile(legit, []byte("glyphs:\n  separator_width: 5\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Manually build path with ".." — filepath.Join would resolve it away.
	sep := string(filepath.Separator)
	traversal := dir + sep + "subdir" + sep + ".." + sep + "legit.yaml"
	th := resolveTheme(traversal)
	if th != nil {
		t.Errorf("expected nil for traversal path containing '..', got %+v", th)
	}
}

func TestResolveTheme_FileTooLarge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.yaml")

	// Write a file just over the 1MB limit.
	data := make([]byte, maxThemeFileSize+1)
	for i := range data {
		data[i] = 'x'
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	th := resolveTheme(path)
	if th != nil {
		t.Errorf("expected nil for oversized file, got %+v", th)
	}
}

func TestResolveTheme_FileExactlyAtLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exact.yaml")

	// Valid YAML payload padded to exactly maxThemeFileSize.
	prefix := []byte("glyphs:\n  separator_width: 7\n# padding: ")
	pad := make([]byte, maxThemeFileSize-len(prefix))
	for i := range pad {
		pad[i] = 'a'
	}
	data := append(prefix, pad...)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	th := resolveTheme(path)
	if th == nil {
		t.Fatal("expected non-nil theme for file exactly at size limit")
	}
}

func TestResolveTheme_DirectoryPath(t *testing.T) {
	// Directories are rejected because resolveTheme only accepts regular files.
	dir := t.TempDir()
	th := resolveTheme(dir)
	if th != nil {
		t.Errorf("expected nil for directory path, got %+v", th)
	}
}
