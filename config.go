package main

import (
	"strings"

	goPackages "golang.org/x/tools/go/packages"
)

type config struct {
	modulePrefix string
	roots        []string
	excludes     []string
}

type stringList []string

func (list *stringList) String() string {
	return strings.Join(*list, ",")
}

func (list *stringList) Set(value string) error {
	*list = append(*list, value)

	return nil
}

func defaultExcludes() []string {
	return []string{"/vendor/", "/node_modules/", "/testdata/"}
}

func detectModulePath(loaded []*goPackages.Package) string {
	for _, pkg := range loaded {
		hasModule := pkg.Module != nil && pkg.Module.Path != ""
		if hasModule {
			return pkg.Module.Path
		}
	}

	return ""
}

func containsAny(pkgPath string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.Contains(pkgPath, pattern) {
			return true
		}
	}

	return false
}

func (cfg *config) isModulePackage(pkgPath string) bool {
	return strings.HasPrefix(pkgPath, cfg.modulePrefix)
}

func (cfg *config) isExcluded(pkgPath string) bool {
	return containsAny(pkgPath, cfg.excludes)
}

func (cfg *config) isRoot(pkgPath string) bool {
	return containsAny(pkgPath, cfg.roots)
}
