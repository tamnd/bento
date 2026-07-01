---
title: "Release notes"
description: "What changed in each bento release."
weight: 40
---

The authoritative, commit-level history lives in [`CHANGELOG.md`](https://github.com/tamnd/bento/blob/main/CHANGELOG.md) and on the [releases page](https://github.com/tamnd/bento/releases).
This page summarises each version.

bento is early, so these notes are also the honest record of what works.
A feature listed here has landed; anything not listed is either in progress or not started.

## v0.1.0

The first release.
bento runs TypeScript and JavaScript, bridges to Go, and builds a static binary.

- **`bento run`** executes a TypeScript or JavaScript file, a project directory, or a `package.json` script. Existing Node.js and Bun projects are the target: bento reads `package.json`, resolves `node_modules`, and implements the common Node and Bun API surface.
- **`go:` imports.** `import { NewReader } from "go:github.com/klauspost/compress/zstd"` pulls a Go package straight into TypeScript. bento resolves the module, generates the bridge, and makes the exported Go symbols callable, with types carried across from the Go signatures. The Go standard library imports the same way.
- **Compile mode.** Fully typed modules are lowered to Go and compiled, so typed hot paths run as native Go instead of interpreted JavaScript. Coverage is a subset of TypeScript today and grows from here; anything not covered still runs in run mode.
- **`bento build`** compiles a program to a single static, cgo-free executable, with cross-compilation through the standard `GOOS` and `GOARCH`.
- **Dependency and workflow commands.** `bento add`, `install`, and `remove` manage `package.json`; `bento test` runs tests; `bento x` runs a package binary without a global install; `bento repl` opens an interactive prompt; `bento version` prints the build.
- **Packaged everywhere.** Archives, `.deb`/`.rpm`/`.apk`, a multi-arch GHCR image, checksums, and a cosign signature. Pure Go, no cgo, one static binary.
