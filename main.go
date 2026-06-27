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

func loadPackages() ([]*goPackages.Package, error) {
	loadMode := goPackages.NeedName | goPackages.NeedFiles | goPackages.NeedSyntax | goPackages.NeedTypes | goPackages.NeedTypesInfo | goPackages.NeedImports | goPackages.NeedDeps | goPackages.NeedModule
	loadConfig := &goPackages.Config{
		Mode:  loadMode,
		Tests: true,
	}

	return goPackages.Load(loadConfig, "./...")
}

func run(cfg *config, fix bool) {
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

	fileSet := pickFileSet(loaded)

	errorCount := reportLoadErrors(loaded)
	if errorCount > 0 {
		fmt.Printf("\n%d load errors — coverage incomplete, referenced symbols may look dead. ", errorCount)

		if fix {
			fmt.Printf("Refusing -fix.\n")

			return
		}

		fmt.Printf("Report is LOW CONFIDENCE.\n\n")
	}

	graph := newAnalyzer(fileSet, cfg)
	graph.build(loaded)
	reached := graph.reachable()

	deadObjects := collectDeadObjects(graph, loaded, reached)
	if !fix {
		reportFindings(fileSet, deadObjects)

		return
	}

	deadPositions := collectDeletablePositions(graph, loaded, deadObjects)

	rewrittenFiles := applyFix(loaded, deadPositions)
	for _, name := range rewrittenFiles {
		fmt.Printf("rewrote %s\n", name)
	}

	fmt.Printf("\ndeleted %d type/var/func declarations across %d files (consts skipped — see report mode)\n", len(deadPositions), len(rewrittenFiles))
}

func main() {
	var (
		roots    stringList
		excludes stringList
	)

	flag.Var(&roots, "root", "package path substring whose exported decls are entry points, always reachable (repeatable)")
	flag.Var(&excludes, "exclude", "package path substring to skip entirely (repeatable; adds to defaults)")
	fix := flag.Bool("fix", false, "delete unused exported types/vars/funcs (consts are reported only)")

	flag.Parse()

	allExcludes := append(defaultExcludes(), excludes...)
	cfg := &config{
		modulePrefix: "",
		roots:        roots,
		excludes:     allExcludes,
	}

	run(cfg, *fix)
}
