package main

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

type analyzer struct {
	fileSet *token.FileSet
	cfg     *config
	edges   map[string][]string
	roots   map[string]bool
}

func newAnalyzer(fileSet *token.FileSet, cfg *config) *analyzer {
	return &analyzer{
		fileSet: fileSet,
		cfg:     cfg,
		edges:   map[string][]string{},
		roots:   map[string]bool{},
	}
}

func (graph *analyzer) positionOf(object types.Object) string {
	return graph.fileSet.Position(object.Pos()).String()
}

func (graph *analyzer) addEdge(fromNodes []string, toNode string) {
	for _, from := range fromNodes {
		graph.edges[from] = append(graph.edges[from], toNode)
	}
}

func (graph *analyzer) collectUses(pkg *packages.Package, node ast.Node, fromNodes []string, asRoot bool) {
	inspect := func(current ast.Node) bool {
		ident, isIdent := current.(*ast.Ident)
		if !isIdent {
			return true
		}

		object := pkg.TypesInfo.Uses[ident]
		unresolved := object == nil || object.Pkg() == nil
		if unresolved || !graph.cfg.isModulePackage(object.Pkg().Path()) {
			return true
		}

		toNode := graph.positionOf(object)
		if asRoot {
			graph.roots[toNode] = true
		} else {
			graph.addEdge(fromNodes, toNode)
		}

		return true
	}
	ast.Inspect(node, inspect)
}

func (graph *analyzer) receiverTypeNode(pkg *packages.Package, receiver *ast.FieldList) string {
	if receiver == nil || len(receiver.List) == 0 {
		return ""
	}

	expression := receiver.List[0].Type

	star, isStar := expression.(*ast.StarExpr)
	if isStar {
		expression = star.X
	}

	ident, isIdent := expression.(*ast.Ident)
	if !isIdent {
		return ""
	}

	object := pkg.TypesInfo.Uses[ident]
	unresolved := object == nil || object.Pkg() == nil
	if unresolved || !graph.cfg.isModulePackage(object.Pkg().Path()) {
		return ""
	}

	return graph.positionOf(object)
}

func isTestEntryPoint(name string) bool {
	prefixes := []string{"Test", "Benchmark", "Fuzz", "Example"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}

	return false
}

func (graph *analyzer) processFunc(pkg *packages.Package, funcDecl *ast.FuncDecl, isTest bool) {
	object := pkg.TypesInfo.Defs[funcDecl.Name]
	if object == nil {
		return
	}

	node := graph.positionOf(object)
	isMain := funcDecl.Name.Name == "main" && pkg.Name == "main"
	isTestEntry := isTest && funcDecl.Recv == nil && isTestEntryPoint(funcDecl.Name.Name)
	isEntryPoint := funcDecl.Name.Name == "init" || isMain || isTestEntry

	if funcDecl.Recv == nil && isEntryPoint {
		graph.roots[node] = true
	}

	if funcDecl.Recv != nil {
		receiverNode := graph.receiverTypeNode(pkg, funcDecl.Recv)
		if receiverNode != "" {
			graph.addEdge([]string{receiverNode}, node)
		}
	}

	graph.collectUses(pkg, funcDecl, []string{node}, isTest)
}

func (graph *analyzer) valueSpecNodes(pkg *packages.Package, names []*ast.Ident) []string {
	nodes := []string{}

	for _, name := range names {
		object := pkg.TypesInfo.Defs[name]
		if object != nil {
			nodes = append(nodes, graph.positionOf(object))
		}
	}

	return nodes
}

func (graph *analyzer) processGen(pkg *packages.Package, genDecl *ast.GenDecl, isTest bool) {
	for _, spec := range genDecl.Specs {
		typeSpec, isType := spec.(*ast.TypeSpec)
		if isType {
			object := pkg.TypesInfo.Defs[typeSpec.Name]
			if object != nil {
				graph.collectUses(pkg, typeSpec.Type, []string{graph.positionOf(object)}, isTest)
			}

			continue
		}

		valueSpec, isValue := spec.(*ast.ValueSpec)
		if !isValue {
			continue
		}

		nodes := graph.valueSpecNodes(pkg, valueSpec.Names)
		if valueSpec.Type != nil {
			graph.collectUses(pkg, valueSpec.Type, nodes, isTest)
		}

		graph.linkEnumType(pkg, valueSpec.Names)

		for _, value := range valueSpec.Values {
			graph.collectUses(pkg, value, nodes, true)
		}
	}
}

func (graph *analyzer) linkEnumType(pkg *packages.Package, names []*ast.Ident) {
	for _, name := range names {
		constObject, isConst := pkg.TypesInfo.Defs[name].(*types.Const)
		if !isConst {
			continue
		}

		namedType, isNamed := constObject.Type().(*types.Named)
		if !isNamed {
			continue
		}

		typeObject := namedType.Obj()
		if typeObject.Pkg() == nil || !graph.cfg.isModulePackage(typeObject.Pkg().Path()) {
			continue
		}

		graph.addEdge([]string{graph.positionOf(typeObject)}, graph.positionOf(constObject))
	}
}

func (graph *analyzer) addRootPackage(pkg *packages.Package) {
	scope := pkg.Types.Scope()
	for _, name := range scope.Names() {
		object := scope.Lookup(name)
		if object.Exported() {
			graph.roots[graph.positionOf(object)] = true
		}
	}
}

func (graph *analyzer) processFile(pkg *packages.Package, file *ast.File) {
	isTest := strings.HasSuffix(graph.fileSet.Position(file.Pos()).Filename, "_test.go")
	for _, decl := range file.Decls {
		funcDecl, isFunc := decl.(*ast.FuncDecl)
		if isFunc {
			graph.processFunc(pkg, funcDecl, isTest)

			continue
		}

		genDecl, isGen := decl.(*ast.GenDecl)
		if isGen {
			graph.processGen(pkg, genDecl, isTest)
		}
	}
}

func (graph *analyzer) build(loaded []*packages.Package) {
	for _, currentPackage := range loaded {
		notLoaded := currentPackage.TypesInfo == nil
		outsideModule := !graph.cfg.isModulePackage(currentPackage.PkgPath)
		skip := notLoaded || outsideModule || graph.cfg.isExcluded(currentPackage.PkgPath)
		if skip {
			continue
		}

		if graph.cfg.isRoot(currentPackage.PkgPath) {
			graph.addRootPackage(currentPackage)
		}

		for _, file := range currentPackage.Syntax {
			graph.processFile(currentPackage, file)
		}
	}
}

func (graph *analyzer) reachable() map[string]bool {
	reached := map[string]bool{}
	queue := []string{}

	for root := range graph.roots {
		if !reached[root] {
			reached[root] = true

			queue = append(queue, root)
		}
	}

	for len(queue) > 0 {
		current := queue[len(queue)-1]
		queue = queue[:len(queue)-1]

		for _, next := range graph.edges[current] {
			if !reached[next] {
				reached[next] = true

				queue = append(queue, next)
			}
		}
	}

	return reached
}

func collectDeadObjects(graph *analyzer, loaded []*packages.Package, reached map[string]bool) []types.Object {
	deadObjects := []types.Object{}
	seenPositions := map[string]bool{}

	for _, pkg := range loaded {
		if !graph.cfg.isModulePackage(pkg.PkgPath) || graph.cfg.isExcluded(pkg.PkgPath) {
			continue
		}

		if graph.cfg.isRoot(pkg.PkgPath) {
			continue
		}

		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			object := scope.Lookup(name)
			if !object.Exported() {
				continue
			}

			position := graph.positionOf(object)
			if reached[position] || seenPositions[position] {
				continue
			}

			seenPositions[position] = true

			deadObjects = append(deadObjects, object)
		}
	}

	return deadObjects
}
