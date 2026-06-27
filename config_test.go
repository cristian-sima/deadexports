package main

import "testing"

func TestStringListAppends(t *testing.T) {
	var list stringList

	if err := list.Set("alpha"); err != nil {
		t.Fatalf("set alpha: %s", err.Error())
	}

	if err := list.Set("beta"); err != nil {
		t.Fatalf("set beta: %s", err.Error())
	}

	if len(list) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(list))
	}

	if list.String() != "alpha,beta" {
		t.Errorf("expected joined \"alpha,beta\", got %q", list.String())
	}
}

func TestConfigMatchers(t *testing.T) {
	cfg := &config{
		modulePrefix: "example.com/app",
		roots:        []string{"/bindings/"},
		excludes:     []string{"/garbage/"},
	}

	if !cfg.isModulePackage("example.com/app/leaf") {
		t.Errorf("expected module package to match prefix")
	}

	if cfg.isModulePackage("other.com/x") {
		t.Errorf("did not expect foreign package to match")
	}

	if !cfg.isRoot("example.com/app/bindings/ui") {
		t.Errorf("expected root pattern to match")
	}

	if !cfg.isExcluded("example.com/app/garbage/dump") {
		t.Errorf("expected exclude pattern to match")
	}

	if cfg.isExcluded("example.com/app/leaf") {
		t.Errorf("did not expect leaf to be excluded")
	}
}
