package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"os"

	"golang.org/x/tools/go/packages"
	goimports "golang.org/x/tools/imports"
)

func pruneImports(filename string, source []byte) []byte {
	processed, err := goimports.Process(filename, source, nil)
	if err != nil {
		return source
	}

	return processed
}

func dropComment(removedComments map[*ast.CommentGroup]bool, group *ast.CommentGroup) {
	if group != nil {
		removedComments[group] = true
	}
}

func receiverTypeIsDead(funcDecl *ast.FuncDecl, deadPositions map[token.Pos]bool, info *types.Info) bool {
	if funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
		return false
	}

	expression := funcDecl.Recv.List[0].Type

	star, isStar := expression.(*ast.StarExpr)
	if isStar {
		expression = star.X
	}

	ident, isIdent := expression.(*ast.Ident)
	if !isIdent {
		return false
	}

	object := info.Uses[ident]
	if object == nil {
		return false
	}

	return deadPositions[object.Pos()]
}

func filterDeadSpecs(genDecl *ast.GenDecl, deadPositions map[token.Pos]bool, removedComments map[*ast.CommentGroup]bool) bool {
	changed := false
	keptSpecs := []ast.Spec{}

	for _, spec := range genDecl.Specs {
		typeSpec, isType := spec.(*ast.TypeSpec)
		if isType && deadPositions[typeSpec.Name.NamePos] {
			changed = true

			dropComment(removedComments, typeSpec.Doc)

			continue
		}

		valueSpec, isValue := spec.(*ast.ValueSpec)
		isSingleDeadValue := isValue && len(valueSpec.Names) == 1 && deadPositions[valueSpec.Names[0].NamePos]
		if isSingleDeadValue {
			changed = true

			dropComment(removedComments, valueSpec.Doc)

			continue
		}

		keptSpecs = append(keptSpecs, spec)
	}

	genDecl.Specs = keptSpecs

	return changed
}

func pruneFileDeclarations(file *ast.File, deadPositions map[token.Pos]bool, info *types.Info) bool {
	changed := false
	removedComments := map[*ast.CommentGroup]bool{}
	keptDecls := []ast.Decl{}

	for _, decl := range file.Decls {
		funcDecl, isFunc := decl.(*ast.FuncDecl)
		if isFunc {
			deadByName := deadPositions[funcDecl.Name.NamePos]

			deadByReceiver := receiverTypeIsDead(funcDecl, deadPositions, info)
			if deadByName || deadByReceiver {
				changed = true

				dropComment(removedComments, funcDecl.Doc)

				continue
			}
		}

		genDecl, isGen := decl.(*ast.GenDecl)
		if isGen {
			if filterDeadSpecs(genDecl, deadPositions, removedComments) {
				changed = true
			}

			if len(genDecl.Specs) == 0 {
				dropComment(removedComments, genDecl.Doc)

				continue
			}
		}

		keptDecls = append(keptDecls, decl)
	}

	file.Decls = keptDecls

	if changed {
		keptComments := []*ast.CommentGroup{}

		for _, group := range file.Comments {
			if !removedComments[group] {
				keptComments = append(keptComments, group)
			}
		}

		file.Comments = keptComments
	}

	return changed
}

func fixOnce(cfg *config, loaded []*packages.Package) int {
	fileSet := pickFileSet(loaded)
	graph := newAnalyzer(fileSet, cfg)
	graph.build(loaded)
	reached := graph.reachable()
	deadObjects := collectDeadObjects(graph, loaded, reached)
	deletable := collectDeletablePositions(graph, deadObjects, reached)
	applyFix(loaded, deletable)

	return len(deletable)
}

func reverseReferences(graph *analyzer) map[string][]string {
	reverse := map[string][]string{}

	for from, targets := range graph.edges {
		for _, target := range targets {
			reverse[target] = append(reverse[target], from)
		}
	}

	return reverse
}

func deletionCandidates(graph *analyzer, deadObjects []types.Object, reached map[string]bool) map[string]token.Pos {
	candidates := map[string]token.Pos{}

	for _, object := range deadObjects {
		_, isConst := object.(*types.Const)
		if isConst && !graph.cfg.pruneEnums {
			continue
		}

		candidates[graph.positionOf(object)] = object.Pos()
	}

	for position, method := range graph.methods {
		if !reached[position] {
			candidates[position] = method.Pos()
		}
	}

	return candidates
}

func hasSurvivingReferencer(referencers []string, candidates map[string]token.Pos) bool {
	for _, referencer := range referencers {
		if _, willDelete := candidates[referencer]; !willDelete {
			return true
		}
	}

	return false
}

func collectDeletablePositions(graph *analyzer, deadObjects []types.Object, reached map[string]bool) map[token.Pos]bool {
	candidates := deletionCandidates(graph, deadObjects, reached)
	reverse := reverseReferences(graph)

	for {
		changed := false

		for position := range candidates {
			if hasSurvivingReferencer(reverse[position], candidates) {
				delete(candidates, position)

				changed = true
			}
		}

		if !changed {
			break
		}
	}

	deletable := map[token.Pos]bool{}
	for _, position := range candidates {
		deletable[position] = true
	}

	return deletable
}

func applyFix(loaded []*packages.Package, deadPositions map[token.Pos]bool) []string {
	rewrittenFiles := []string{}
	rewritten := map[string]bool{}

	for _, pkg := range loaded {
		for _, file := range pkg.Syntax {
			if !pruneFileDeclarations(file, deadPositions, pkg.TypesInfo) {
				continue
			}

			position := pkg.Fset.Position(file.Pos())
			if rewritten[position.Filename] {
				continue
			}

			buffer := &bytes.Buffer{}
			if err := format.Node(buffer, pkg.Fset, file); err != nil {
				fmt.Printf("format %s: %s\n", position.Filename, err.Error())

				continue
			}

			output := pruneImports(position.Filename, buffer.Bytes())

			if err := os.WriteFile(position.Filename, output, 0o644); err != nil {
				fmt.Printf("write %s: %s\n", position.Filename, err.Error())

				continue
			}

			rewritten[position.Filename] = true

			rewrittenFiles = append(rewrittenFiles, position.Filename)
		}
	}

	return rewrittenFiles
}
