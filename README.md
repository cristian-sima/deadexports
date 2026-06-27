# deadexports

A whole-module dead-export finder for Go.

`deadexports` reports **exported** declarations (types, funcs, vars, consts)
that are unreachable across your entire module. These are the dead exports that
per-package linters deliberately leave alone — because, looking at one package
in isolation, they cannot prove some other importer doesn't use the symbol.
`deadexports` loads the whole module at once and proves nothing does.

## The problem it solves

In an application (as opposed to a published library), an exported identifier
that nothing in the module references is just dead weight. But:

- **staticcheck `unused` (U1000)** conservatively **skips exported
  package-level identifiers** — in per-package analysis they might be part of a
  public API, so it never flags them. Dead exports in your app's leaf packages
  slip through.
- **`golang.org/x/tools/cmd/deadcode`** is whole-program, but only finds
  unreachable **functions** — not dead types, vars, or consts — and has no
  notion of framework entry points.

So a leaf package full of exported-but-unused types, vars, and consts shows
clean under the usual linters. `deadexports` catches them.

## How it works

It loads every package in the module (`./...`), builds a reachability graph
over exported declarations, seeds it from roots (`main`, `init`, test entry
points, and any packages you mark as framework roots), then reports every
exported declaration the graph never reaches.

## Comparison

| Tool | Scope | Catches | Framework roots |
|------|-------|---------|-----------------|
| staticcheck `unused` | per-package | unexported, plus exported only with whole-program mode | no |
| `x/tools/cmd/deadcode` | whole-program | functions only | no |
| **deadexports** | **whole-module** | **types, funcs, vars, consts** | **`-root` patterns** |

## Install

```sh
go install github.com/cristian-sima/deadexports@latest
```

## Usage

Run from the module root:

```sh
deadexports
```

Flags:

| Flag | Repeatable | Meaning |
|------|-----------|---------|
| `-root <substring>` | yes | package-path substring whose exported decls are entry points (called by a framework via reflection/codegen), always treated as reachable and never reported |
| `-exclude <substring>` | yes | package-path substring to skip entirely; adds to the built-in defaults (`/vendor/`, `/node_modules/`, `/testdata/`) |
| `-fix` | — | delete unused exported types/vars/funcs in place (consts are reported only, never deleted) |

Example for a Wails app whose `backend/bindings/` packages are invoked from the
frontend by reflection, with a gitignored dump folder:

```sh
deadexports -root backend/bindings/ -exclude garbage
```

## What counts as reachable

- `main` and `init` functions.
- `Test*` / `Benchmark*` / `Fuzz*` / `Example*` functions in `_test.go` files,
  and anything they reference (symbols used only by tests are kept).
- Every exported declaration in a `-root` package, plus everything those
  declarations reference.
- Methods of a reachable type (so a method that only satisfies an interface is
  not flagged); a method of a dead type is removed together with the type under
  `-fix`.
- Enum members reachable via their named type, even when consumed only by value.

## Behavior matrix

Each case is covered by a positive test (correctly flagged dead) and a negative
test (correctly kept):

| Case | Flagged dead | Kept |
|------|--------------|------|
| Exported func unused module-wide | yes | reached-from-`main` func |
| Cross-package exported unused | leaf export used by nobody | leaf export used by another package |
| Exported type + methods | unused type (and its methods, under `-fix`) | type used somewhere (and its methods) |
| Enum via named type | whole enum whose type is unused | members when the type is used |
| Exported var | unused var | referenced var |
| `-root` package | exports reported without the flag | exports kept with `-root` |
| `-exclude` package | reported without the flag | skipped with `-exclude` |
| Test-only references | func used by nothing | func used only in `_test.go` |
| `-fix` safety | deletes dead func/type | keeps live; never deletes consts |

## Caveats

- **Methods are not reported on their own** — only the package-scope decls
  (types, funcs, vars, consts). A dead method surfaces through its dead type.
- **Consts are reported, never auto-deleted** — `iota`/enum value semantics make
  blind deletion unsafe; review them by hand.
- **Load errors lower confidence** — if some packages fail to type-check,
  referenced symbols can look dead. `deadexports` says so and refuses `-fix`.
- Run it from the module root; analysis is always over `./...`.

## License

[MIT](LICENSE)
