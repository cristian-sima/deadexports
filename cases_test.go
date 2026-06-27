package main

import "testing"

const singlePackageSample = `package main

type Color uint8

const (
	Red Color = iota
	Green
	Blue
)

type Box struct {
	Shade Color
}

func Build() Box { return Box{} }

func (box Box) Area() int { return 0 }

func Unused() {}

type Orphan struct{}

func (orphan Orphan) Hello() string { return "" }

var Used = 5

var Dead = 7

type Mode uint8

const (
	ModeOff Mode = iota
	ModeOn
)

func main() {
	_ = Build()
	_ = Used
}
`

func TestSinglePackageReachability(t *testing.T) {
	files := map[string]string{"sample.go": singlePackageSample}
	dead := deadNamesFor(t, files, &config{}, false)

	assertDead(t, dead, "Unused", "Orphan", "Dead")
	assertLive(t, dead, "Build", "Box", "Area", "Color", "Red", "Green", "Blue", "Used")
}

func TestDeadEnumWholeChain(t *testing.T) {
	files := map[string]string{"sample.go": singlePackageSample}
	dead := deadNamesFor(t, files, &config{}, false)

	assertDead(t, dead, "Mode", "ModeOff", "ModeOn")
}

func TestTestOnlyReferenceIsRoot(t *testing.T) {
	files := map[string]string{
		"main.go": `package main

func OnlyTest() {}

func NeverUsed() {}

func main() {}
`,
		"main_test.go": `package main

import "testing"

func TestOnlyTest(t *testing.T) {
	OnlyTest()
}
`,
	}
	dead := deadNamesFor(t, files, &config{}, true)

	assertLive(t, dead, "OnlyTest", "TestOnlyTest")
	assertDead(t, dead, "NeverUsed")
}
