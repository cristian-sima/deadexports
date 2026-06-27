package main

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/packages"
)

const sampleModulePath = "example.com/sample"

func writeSource(t *testing.T, dir, rel, content string) {
	t.Helper()

	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %s", rel, err.Error())
	}

	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %s", rel, err.Error())
	}
}

func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	writeSource(t, dir, "go.mod", "module "+sampleModulePath+"\n\ngo 1.21\n")

	for rel, content := range files {
		writeSource(t, dir, rel, content)
	}

	return dir
}

func loadModule(t *testing.T, dir string, withTests bool) []*packages.Package {
	t.Helper()

	loadMode := packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
		packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps
	loadConfig := &packages.Config{
		Mode:  loadMode,
		Dir:   dir,
		Tests: withTests,
	}

	loaded, err := packages.Load(loadConfig, "./...")
	if err != nil {
		t.Fatalf("load: %s", err.Error())
	}

	return loaded
}

func deadNamesFor(t *testing.T, files map[string]string, cfg *config, withTests bool) map[string]bool {
	t.Helper()

	if cfg.modulePrefix == "" {
		cfg.modulePrefix = sampleModulePath
	}

	if cfg.excludes == nil {
		cfg.excludes = defaultExcludes()
	}

	dir := writeModule(t, files)
	loaded := loadModule(t, dir, withTests)
	fileSet := pickFileSet(loaded)
	graph := newAnalyzer(fileSet, cfg)
	graph.build(loaded)
	reached := graph.reachable()
	deadObjects := collectDeadObjects(graph, loaded, reached)

	names := map[string]bool{}
	for _, object := range deadObjects {
		names[object.Name()] = true
	}

	return names
}

func assertDead(t *testing.T, dead map[string]bool, names ...string) {
	t.Helper()

	for _, name := range names {
		if !dead[name] {
			t.Errorf("expected %q reported DEAD, but it was kept (dead=%v)", name, dead)
		}
	}
}

func assertLive(t *testing.T, dead map[string]bool, names ...string) {
	t.Helper()

	for _, name := range names {
		if dead[name] {
			t.Errorf("expected %q kept LIVE, but it was reported dead (dead=%v)", name, dead)
		}
	}
}
