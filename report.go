package main

import (
	"fmt"
	"go/token"
	"go/types"
	"sort"

	goPackages "golang.org/x/tools/go/packages"
)

type finding struct {
	kind     string
	name     string
	position string
}

func describeObject(object types.Object) string {
	switch object.(type) {
	case *types.TypeName:
		return "type"
	case *types.Const:
		return "const"
	case *types.Var:
		return "var"
	case *types.Func:
		return "func"
	default:
		return "decl"
	}
}

func reportFindings(fileSet *token.FileSet, deadObjects []types.Object) {
	findings := make([]finding, 0, len(deadObjects))

	for _, object := range deadObjects {
		entry := finding{
			kind:     describeObject(object),
			name:     object.Name(),
			position: fileSet.Position(object.Pos()).String(),
		}
		findings = append(findings, entry)
	}

	lessFinding := func(left, right int) bool {
		return findings[left].position < findings[right].position
	}
	sort.Slice(findings, lessFinding)

	countByKind := map[string]int{}

	for _, entry := range findings {
		marker := ""
		if entry.kind == "const" {
			marker = "\t(const: manual review — iota/enum value semantics, not auto-deleted)"
		}

		fmt.Printf("%s\t%s %s%s\n", entry.position, entry.kind, entry.name, marker)
		countByKind[entry.kind]++
	}

	fmt.Printf("\n%d unused exported declarations (type=%d const=%d var=%d func=%d)\n",
		len(findings), countByKind["type"], countByKind["const"], countByKind["var"], countByKind["func"])
}

func reportLoadErrors(loaded []*goPackages.Package) int {
	errorCount := 0
	reportError := func(pkg *goPackages.Package) {
		for _, loadError := range pkg.Errors {
			errorCount++

			fmt.Printf("LOAD ERROR [%s]: %s\n", pkg.PkgPath, loadError.Error())
		}
	}
	goPackages.Visit(loaded, nil, reportError)

	return errorCount
}

func pickFileSet(loaded []*goPackages.Package) *token.FileSet {
	for _, pkg := range loaded {
		if pkg.Fset != nil {
			return pkg.Fset
		}
	}

	return token.NewFileSet()
}
