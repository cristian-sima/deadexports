// Command deadexports reports exported declarations that are unreachable
// across the whole module. These are the dead exports that per-package linters
// such as staticcheck's "unused" deliberately skip, because in isolation an
// external importer might use them; loading the whole module proves nothing
// does.
package main

import (
	"flag"
	"fmt"

	goPackages "golang.org/x/tools/go/packages"
)

const maxFixPasses = 25

func loadPackages() ([]*goPackages.Package, error) {
	loadMode := goPackages.NeedName | goPackages.NeedFiles | goPackages.NeedSyntax | goPackages.NeedTypes | goPackages.NeedTypesInfo | goPackages.NeedImports | goPackages.NeedDeps | goPackages.NeedModule
	loadConfig := &goPackages.Config{
		Mode:  loadMode,
		Tests: true,
	}

	return goPackages.Load(loadConfig, "./...")
}

func reportOnce(cfg *config, loaded []*goPackages.Package) {
	fileSet := pickFileSet(loaded)
	graph := newAnalyzer(fileSet, cfg)
	graph.build(loaded)
	reached := graph.reachable()
	deadObjects := collectDeadObjects(graph, loaded, reached)
	reportFindings(fileSet, deadObjects)
}

func runFixPasses(cfg *config, loaded []*goPackages.Package, loop bool) {
	maxPasses := 1
	if loop {
		maxPasses = maxFixPasses
	}

	total := 0
	for pass := 0; pass < maxPasses; pass++ {
		if pass > 0 {
			reloaded, err := loadPackages()
			if err != nil {
				fmt.Printf("load: %s\n", err.Error())

				break
			}

			loaded = reloaded
		}

		deleted := fixOnce(cfg, loaded)
		total += deleted

		fmt.Printf("pass %d: deleted %d declarations\n", pass+1, deleted)

		if deleted == 0 {
			break
		}

		exhausted := loop && pass+1 == maxPasses
		if exhausted {
			fmt.Printf("reached max %d passes — stopping (may not be a fixpoint)\n", maxPasses)
		}
	}

	fmt.Printf("\ntotal: deleted %d declarations\n", total)
}

func run(cfg *config, fix bool, loop bool) {
	loaded, err := loadPackages()
	if err != nil {
		fmt.Printf("load: %s\n", err.Error())

		return
	}

	cfg.modulePrefix = detectModulePath(loaded)
	if cfg.modulePrefix == "" {
		fmt.Printf("could not determine module path — run from the module root\n")

		return
	}

	errorCount := reportLoadErrors(loaded)
	if errorCount > 0 {
		fmt.Printf("\n%d load errors — coverage incomplete, referenced symbols may look dead. ", errorCount)

		if fix {
			fmt.Printf("Refusing -fix.\n")

			return
		}

		fmt.Printf("Report is LOW CONFIDENCE.\n\n")
	}

	if !fix {
		reportOnce(cfg, loaded)

		return
	}

	runFixPasses(cfg, loaded, loop)
}

func main() {
	var (
		roots    stringList
		excludes stringList
	)

	flag.Var(&roots, "root", "package path substring whose exported decls are entry points, always reachable (repeatable)")
	flag.Var(&excludes, "exclude", "package path substring to skip entirely (repeatable; adds to defaults)")
	fix := flag.Bool("fix", false, "delete unused exported types/vars/funcs (consts are reported only)")
	loop := flag.Bool("loop", false, "with -fix, repeat until a pass deletes nothing (fixpoint)")
	includeUnexported := flag.Bool("include-unexported", false, "also report/delete dead unexported declarations (cruder than staticcheck on reflection and build tags)")

	flag.Parse()

	allExcludes := append(defaultExcludes(), excludes...)
	wantUnexported := *includeUnexported
	cfg := &config{
		modulePrefix:      "",
		roots:             roots,
		excludes:          allExcludes,
		includeUnexported: wantUnexported,
	}

	run(cfg, *fix, *loop)
}
