# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What Dud is

Dud is a Go CLI for versioning large data files/directories alongside source
code and for running data pipelines (a lighter, faster take on DVC). Design
goals from CONTRIBUTING.md: **simple, fast, transparent** — do one thing well,
never mutate the workspace implicitly, keep state human-readable. Commits only
happen on explicit `dud commit`; checkouts are symlinks by default; remote
storage is delegated entirely to `rclone`; no analytics, ever.

## Commands

Module: `github.com/kevin-hanselman/dud`, Go 1.21. All Go code lives under
`src/`; `main.go` is a thin entry point that calls `cmd.Main()`.

```sh
make test              # gofumpt + goimports + go vet + staticcheck, then go test -cover -race ./...
make test-short        # same but `go test -short ./...`
make lint              # go vet + staticcheck only
make fmt               # goimports -w + gofumpt -w (CI's golangci-lint also runs; see `make deep-lint`)
make dud               # builds ./dud (runs `make test` first); plain `go build -o dud .` skips tests
make install           # copies ./dud to $GOBIN — required before integration tests
make integration-test  # python integration/run_tests.py (needs dud on PATH, plus rclone, tree, graphviz)
make bench             # go test ./... -benchmem -bench .
make src/mocks         # regenerate src/mocks/Cache.go with mockery (only mock is the Cache interface)
make cli-docs          # ./dud gen-docs hugo/content/cli (Cobra-generated CLI reference)
```

Single Go test: `go test ./src/cache -run TestCommitDirectory` (add `-v`,
`-race` as needed). Single integration test:
`python integration/run_tests.py integration/tests/basic_run`.
`--pin` overwrites a test's `expected_fs.txt`/`expected_output.txt` with
current output — use it deliberately after verifying the new behaviour.

The official dev environment is the Docker image in `integration/Dockerfile`
(`make docker` for a shell; any Makefile rule can be run inside it as
`make docker-<rule>`, e.g. `make docker-integration-test`). Working natively
is fine if you have Go, rclone, tree and graphviz installed.

## Architecture

Package dependency flow (no cycles): `cmd` → `index` → `stage` → `artifact` →
`fsutil`/`checksum`. `cache` sits beside `index` and is consumed by it.

- **`src/artifact`** — `Artifact` is a tracked file or directory: `Path`
  (always relative to project root), `Checksum`, `IsDir`, `DisableRecursion`,
  `SkipCache`. `artifact.Status` describes an artifact relative to both the
  workspace and the cache. `UnmarshalJSON` keeps compatibility with the
  pre-struct-tag on-disk schema (see integration test `old_dir_manifest_schema`).
- **`src/stage`** — `Stage` = one YAML stage file: `Command`, `WorkingDir`,
  `Inputs`, `Outputs` (maps keyed by artifact path), and a `Checksum` of the
  stage *definition* (excluding artifact checksums) used to detect user edits.
  `FromFile`/`ToFile` translate between the in-memory form and the YAML form
  (paths become map keys; `SkipCache` is forced true for all inputs on load and
  hidden on write). YAML decoding is strict.
- **`src/index`** — `Index` is `map[stagePath]*Stage`, the whole project DAG.
  `.dud/index` on disk is just a newline-separated list of stage paths; stages
  are re-read from their YAML files on every load. Each operation
  (`Commit`, `Checkout`, `Status`, `Run`, `Fetch`, `Push`, `Graph`) is a method
  that recurses upstream via `findOwner` (which stage owns an input artifact),
  using `visited`/`inProgress` maps for memoisation and cycle detection. Edges
  are implicit: a stage's input that is another stage's output.
- **`src/cache`** — `Cache` interface + `LocalCache` (content-addressed
  directory: `<checksum[:2]>/<checksum[2:]>`, files stored read-only `0444`).
  Directory artifacts are committed as a JSON *directory manifest* stored in
  the cache like any other object, with recursive concurrent workers
  (`maxSharedWorkers`/`maxDedicatedWorkers`). `Fetch`/`Push` shell out to
  `rclone`. The `--config` path is resolved by `resolveRcloneConfig` in
  `cmd/root.go`: explicit `rclone_config` key wins, else project's
  `.dud/rclone.conf` if present, else the flag is omitted so rclone uses its
  own default resolution. `strategy.CheckoutStrategy` selects symlink
  (default) vs copy on checkout.
- **`src/checksum`** — BLAKE3 hashing with pooled buffers/hashers; this is the
  hot path for large datasets, so keep allocations out of it.
- **`src/cmd`** — Cobra commands. `prepare()` in `root.go` is the common
  bootstrap: find project root (walk up to a `.dud/` dir), rewrite CLI paths
  relative to it, `chdir` there, take the `.dud/lock` file (O_EXCL; commands
  must release it via `fatal()`/`unlockProject()`), merge user
  (`$XDG_CONFIG_HOME/dud/config.yaml`) and project (`.dud/config.yaml`) config
  via viper, open the cache, load the index. All logging goes through the
  package-level `agglog.AggLogger` (`Error`/`Info`/`Debug`).

Project layout on disk: `.dud/index`, `.dud/lock`, `.dud/config.yaml`,
`.dud/rclone.conf` (project-local rclone config, optional if `rclone_config`
points elsewhere), and the cache at `.dud/cache` (configurable via `cache`
key).

## Testing conventions

- Unit tests use `testify` (`assert`/`require`/`mock`) and `go-cmp` for diffs;
  `src/testutil` provides `MockFileInfo` and `CreateTempDirs()` for tests that
  touch a real cache + workspace. Package-private function variables (e.g.
  `stage.fromYamlFile`, `index.runCommand`) exist specifically so tests can
  swap them out.
- Integration tests (`integration/tests/<name>/run.sh`) act like a user typing
  commands; subdirectories under a test share one project and run in
  lexicographic order (`00_commit`, `01_checkout`, …). `expected_fs.txt` is a
  `tree` listing of the resulting project and `expected_output.txt` the stdout;
  both are diffed when present. Tests run under `/tmp/dud_integration_tests`
  with a fixed umask so listings are reproducible.
- Progress bars are suppressed when stderr is not a TTY, which is why
  integration tests can diff output.

## Docs and release

- Website is Hugo (`hugo/`, theme is a git submodule: `make submodule-update`).
  CLI docs are generated by `make cli-docs`; other pages are hand-written or
  converted from Jupyter notebooks (`make hugo/content/%.md`).
- Releases are cut via the GitHub UI (tag + curated notes); GoReleaser then
  builds linux/darwin amd64/arm64 binaries and injects `main.version`.
