package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFixDeletesDeadFuncKeepsLiveAndConst(t *testing.T) {
	source := `package main

const DeadConst = 3

func DeadFunc() {}

type DeadType struct{}

func (deadType DeadType) Method() {}

func Kept() {}

func main() { Kept() }
`
	files := map[string]string{"main.go": source}
	dir := writeModule(t, files)
	loaded := loadModule(t, dir, false)
	fileSet := pickFileSet(loaded)
	cfg := &config{
		modulePrefix: sampleModulePath,
		excludes:     defaultExcludes(),
	}
	graph := newAnalyzer(fileSet, cfg)
	graph.build(loaded)
	reached := graph.reachable()
	deadObjects := collectDeadObjects(graph, loaded, reached)

	deadPositions := collectDeadPositions(deadObjects)
	rewritten := applyFix(loaded, deadPositions)
	if len(rewritten) != 1 {
		t.Fatalf("expected 1 rewritten file, got %d", len(rewritten))
	}

	content, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatalf("read back: %s", err.Error())
	}
	text := string(content)

	if strings.Contains(text, "DeadFunc") {
		t.Errorf("expected DeadFunc deleted, still present:\n%s", text)
	}

	if strings.Contains(text, "DeadType") || strings.Contains(text, "Method") {
		t.Errorf("expected dead type and its method deleted, still present:\n%s", text)
	}

	if !strings.Contains(text, "func Kept") {
		t.Errorf("expected Kept retained:\n%s", text)
	}

	if !strings.Contains(text, "DeadConst") {
		t.Errorf("expected DeadConst retained (consts never auto-deleted):\n%s", text)
	}
}
