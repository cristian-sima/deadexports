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
)

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

func collectDeadPositions(deadObjects []types.Object) map[token.Pos]bool {
	deadPositions := map[token.Pos]bool{}

	for _, object := range deadObjects {
		_, isConst := object.(*types.Const)
		if isConst {
			continue
		}

		deadPositions[object.Pos()] = true
	}

	return deadPositions
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

			if err := os.WriteFile(position.Filename, buffer.Bytes(), 0o644); err != nil {
				fmt.Printf("write %s: %s\n", position.Filename, err.Error())

				continue
			}

			rewritten[position.Filename] = true

			rewrittenFiles = append(rewrittenFiles, position.Filename)
		}
	}

	return rewrittenFiles
}
