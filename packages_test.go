package main

import "testing"

func multiPackageFiles() map[string]string {
	return map[string]string{
		"main.go": `package main

import "example.com/sample/leaf"

func main() {
	leaf.Live()
}
`,
		"leaf/leaf.go": `package leaf

func Live() {}

func Dead() {}
`,
		"api/api.go": `package api

func Handler() {}
`,
		"skip/skip.go": `package skip

func Junk() {}
`,
	}
}

func TestCrossPackageDeadExport(t *testing.T) {
	dead := deadNamesFor(t, multiPackageFiles(), &config{}, false)

	assertDead(t, dead, "Dead", "Handler", "Junk")
	assertLive(t, dead, "Live")
}

func TestRootPackageSuppressed(t *testing.T) {
	cfg := &config{roots: []string{"/api"}}
	dead := deadNamesFor(t, multiPackageFiles(), cfg, false)

	assertLive(t, dead, "Handler", "Live")
	assertDead(t, dead, "Dead", "Junk")
}

func TestExcludedPackageSkipped(t *testing.T) {
	cfg := &config{excludes: append(defaultExcludes(), "/skip")}
	dead := deadNamesFor(t, multiPackageFiles(), cfg, false)

	assertLive(t, dead, "Junk", "Live")
	assertDead(t, dead, "Dead", "Handler")
}
