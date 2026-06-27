package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFixLoopClearsDeadChain(t *testing.T) {
	source := `package main

func Top() { mid() }

func mid() { Leaf() }

func Leaf() {}

func main() {}
`
	files := map[string]string{"main.go": source}
	dir := writeModule(t, files)
	cfg := &config{
		modulePrefix:      sampleModulePath,
		excludes:          defaultExcludes(),
		includeUnexported: true,
	}

	passes := 0
	for passes < 10 {
		loaded := loadModule(t, dir, false)
		deleted := fixOnce(cfg, loaded)
		if deleted == 0 {
			break
		}

		passes++
	}

	content, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatalf("read back: %s", err.Error())
	}
	text := string(content)

	for _, name := range []string{"Top", "mid", "Leaf"} {
		if strings.Contains(text, "func "+name) {
			t.Errorf("expected %s removed after loop, still present:\n%s", name, text)
		}
	}

	if !strings.Contains(text, "func main") {
		t.Errorf("main must remain:\n%s", text)
	}
}

func TestPruneEnumsDeletesWholeDeadEnum(t *testing.T) {
	source := `package main

type Color uint8

const (
	Red Color = iota
	Green
	Blue
)

func main() {}
`
	names := []string{"Color", "Red", "Green", "Blue"}

	keptDir := writeModule(t, map[string]string{"main.go": source})
	keptLoaded := loadModule(t, keptDir, false)
	keptCfg := &config{
		modulePrefix: sampleModulePath,
		excludes:     defaultExcludes(),
	}
	fixOnce(keptCfg, keptLoaded)
	keptText := readMainGo(t, keptDir)

	for _, name := range names {
		if !strings.Contains(keptText, name) {
			t.Errorf("without -prune-enums, %s must be kept:\n%s", name, keptText)
		}
	}

	prunedDir := writeModule(t, map[string]string{"main.go": source})
	prunedLoaded := loadModule(t, prunedDir, false)
	prunedCfg := &config{
		modulePrefix: sampleModulePath,
		excludes:     defaultExcludes(),
		pruneEnums:   true,
	}
	fixOnce(prunedCfg, prunedLoaded)
	prunedText := readMainGo(t, prunedDir)

	for _, name := range names {
		if strings.Contains(prunedText, name) {
			t.Errorf("with -prune-enums, whole-dead enum member %s must be deleted:\n%s", name, prunedText)
		}
	}

	if !strings.Contains(prunedText, "func main") {
		t.Errorf("main must remain:\n%s", prunedText)
	}
}

func TestPruneEnumsKeepsLoneConst(t *testing.T) {
	source := `package main

const LoneCode = "abc"

func main() {}
`
	files := map[string]string{"main.go": source}
	dir := writeModule(t, files)
	loaded := loadModule(t, dir, false)
	cfg := &config{
		modulePrefix: sampleModulePath,
		excludes:     defaultExcludes(),
		pruneEnums:   true,
	}
	fixOnce(cfg, loaded)
	text := readMainGo(t, dir)

	if !strings.Contains(text, "LoneCode") {
		t.Errorf("lone untyped const must be kept even with -prune-enums:\n%s", text)
	}
}

func readMainGo(t *testing.T, dir string) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatalf("read back: %s", err.Error())
	}

	return string(content)
}

func TestFixDeletesMutualTypeCluster(t *testing.T) {
	source := `package main

type Node struct{ edge *Edge }

type Edge struct{ node *Node }

func main() {}
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
	deletable := collectDeletablePositions(graph, deadObjects, reached)
	applyFix(loaded, deletable)
	text := readMainGo(t, dir)

	for _, name := range []string{"Node", "Edge"} {
		if strings.Contains(text, "type "+name) {
			t.Errorf("expected mutual-cluster type %s deleted:\n%s", name, text)
		}
	}
}

func TestFixDeletesDeadTypeWithMethod(t *testing.T) {
	source := `package main

type Box struct{}

func (box Box) Size() int { return 0 }

func main() {}
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
	deletable := collectDeletablePositions(graph, deadObjects, reached)
	applyFix(loaded, deletable)
	text := readMainGo(t, dir)

	for _, fragment := range []string{"type Box", "func (box Box) Size"} {
		if strings.Contains(text, fragment) {
			t.Errorf("expected %q deleted (dead type + its method):\n%s", fragment, text)
		}
	}
}

func TestFixKeepsReferencedDeadExport(t *testing.T) {
	source := `package main

type Widget struct{ Size int }

func consume() { _ = Widget{} }

func main() {}
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

	deletable := collectDeletablePositions(graph, deadObjects, reached)
	applyFix(loaded, deletable)

	content, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatalf("read back: %s", err.Error())
	}
	text := string(content)

	if !strings.Contains(text, "Widget") {
		t.Errorf("expected Widget kept (still referenced by consume), got:\n%s", text)
	}

	if !strings.Contains(text, "func consume") {
		t.Errorf("expected consume retained:\n%s", text)
	}
}

func TestFixPrunesUnusedImports(t *testing.T) {
	source := `package main

import "strings"

func Dead() string { return strings.ToUpper("x") }

func main() {}
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
	deletable := collectDeletablePositions(graph, deadObjects, reached)
	applyFix(loaded, deletable)

	content, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatalf("read back: %s", err.Error())
	}
	text := string(content)

	if strings.Contains(text, "func Dead") {
		t.Errorf("expected Dead deleted:\n%s", text)
	}

	if strings.Contains(text, "strings") {
		t.Errorf("expected now-unused import \"strings\" pruned:\n%s", text)
	}
}

func TestFixDeletesDeadFuncKeepsLiveAndConst(t *testing.T) {
	source := `package main

const DeadConst = 3

func DeadFunc() {}

type DeadType struct{}

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

	deletable := collectDeletablePositions(graph, deadObjects, reached)
	rewritten := applyFix(loaded, deletable)
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

	if strings.Contains(text, "DeadType") {
		t.Errorf("expected method-less dead type deleted, still present:\n%s", text)
	}

	if !strings.Contains(text, "func Kept") {
		t.Errorf("expected Kept retained:\n%s", text)
	}

	if !strings.Contains(text, "DeadConst") {
		t.Errorf("expected DeadConst retained (consts never auto-deleted):\n%s", text)
	}
}
