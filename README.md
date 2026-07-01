# bento

[![ci](https://github.com/tamnd/bento/actions/workflows/ci.yml/badge.svg)](https://github.com/tamnd/bento/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/tamnd/bento)](https://github.com/tamnd/bento/releases/latest)
[![Go Reference](https://pkg.go.dev/badge/github.com/tamnd/bento.svg)](https://pkg.go.dev/github.com/tamnd/bento)
[![Go Report Card](https://goreportcard.com/badge/github.com/tamnd/bento)](https://goreportcard.com/report/github.com/tamnd/bento)
[![License](https://img.shields.io/github/license/tamnd/bento)](./LICENSE)

**bento** is a TypeScript runtime built in Go, a Bun alternative.
It runs your existing Node.js and Bun code unchanged, compiles the typed parts of your TypeScript to Go for speed, and lets you reach into any Go library straight from TypeScript.
The whole thing is pure Go with zero cgo, so it ships as one static binary that cross-compiles to every platform.

The JS engine is [modernc.org/quickjs](https://pkg.go.dev/modernc.org/quickjs), a pure-Go ES2023 engine, so there is no V8 to build and nothing native to link.
That is what keeps bento a single file you can drop onto any machine and run.

Full docs and guides live at **[bento.tamnd.com](https://bento.tamnd.com)**.

## Install

```bash
go install github.com/tamnd/bento/cmd/bento@latest
```

Prefer a prebuilt binary? Grab an archive, a `.deb`/`.rpm`/`.apk`, or a checksum from [releases](https://github.com/tamnd/bento/releases).
A Homebrew tap and a Scoop bucket ship with each release once they are wired up:

```bash
# Homebrew (macOS)
brew install tamnd/tap/bento

# Scoop (Windows)
scoop bucket add tamnd https://github.com/tamnd/scoop-bucket
scoop install bento
```

## Quick start

Write a TypeScript file and run it:

```typescript
// app.ts
const name = "world";
console.log(`hello, ${name}`);
```

```bash
bento run app.ts
```

Reach into a Go library from TypeScript with a `go:` import.
You get the speed and the ecosystem of Go without leaving your script:

```typescript
import { Sum } from "go:github.com/tamnd/bento/examples/mathx";

console.log(Sum([1, 2, 3, 4])); // 10
```

## Why bento

Drop-in compatibility.
Your Node.js and Bun code runs as is, so you are not rewriting anything to try it.

Any Go library from TypeScript.
A `go:` import gives you the whole Go module ecosystem from a `.ts` file, with types carried across the boundary.

Speed where it counts.
The typed parts of your TypeScript compile down to Go, so the hot paths run as compiled code instead of interpreted script.

One static binary, zero cgo.
The runtime and its JS engine are pure Go, so bento cross-compiles to every GOOS and GOARCH and installs as a single file with nothing to link and nothing to download at runtime.

## Status

bento is early.
The runtime, the CLI, and the `go:` import bridge are under active development, and not every Node or Bun API is in place yet.
Track what runs today at [bento.tamnd.com](https://bento.tamnd.com) and in the [releases](https://github.com/tamnd/bento/releases).
If something you rely on is missing, open an issue.

## License

MIT. See [LICENSE](./LICENSE).
