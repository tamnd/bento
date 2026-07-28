# bento

[![ci](https://github.com/tamnd/bento/actions/workflows/ci.yml/badge.svg)](https://github.com/tamnd/bento/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/tamnd/bento)](https://github.com/tamnd/bento/releases/latest)
[![Go Reference](https://pkg.go.dev/badge/github.com/tamnd/bento.svg)](https://pkg.go.dev/github.com/tamnd/bento)
[![Go Report Card](https://goreportcard.com/badge/github.com/tamnd/bento)](https://goreportcard.com/report/github.com/tamnd/bento)
[![License](https://img.shields.io/github/license/tamnd/bento)](./LICENSE)

**bento** is a TypeScript runtime built in Go, a Bun alternative.
It runs your existing Node.js and Bun code unchanged, compiles the typed parts of your TypeScript to Go for speed, and lets you reach into any Go library straight from TypeScript.
The whole thing is pure Go with zero cgo, so it ships as one static binary that cross-compiles to Linux, macOS, Windows and FreeBSD without a per-OS toolchain.

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

`bento run` needs only bento.
`bento build` lowers your code to Go and compiles it, so it needs a Go toolchain on your `PATH` and, for now, a checkout of this repository too: the generated program links against bento's runtime packages, and an installed binary carries no copy of them.
Run inside the checkout or point `BENTO_MODULE_ROOT` at it.
That is [#765](https://github.com/tamnd/bento/issues/765), and it is not how it should stay.

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
The runtime and its JS engine are pure Go, so bento cross-compiles to every platform below and installs as a single file with nothing to link and nothing to download at runtime.

## Platforms

Every release ships `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`, `windows/arm64`, `freebsd/amd64` and `freebsd/arm64`.
CI cross-builds all eight on every pull request, so a target that ships is a target something builds.

64-bit only.
A JavaScript array index runs to 2^53 - 1 and the value runtime holds that bound in an `int`, so there is no 32-bit build to ship: the length is what indexes a Go slice, and a wider constant would not change that.

Linux, macOS and Windows all run the test suite on every pull request, and Linux and Windows also run the ahead-of-time equivalence suite, the slower half that compiles each fixture into a real binary and executes it.
Every pull request unpacks the shipped Windows zip and Linux tarball, runs the binary inside, and compiles a program with it.
Windows is held to the same bar as the other two rather than to a list of the packages that happened to pass.

## Status

bento is early.
The runtime, the CLI, and the `go:` import bridge are under active development, and not every Node or Bun API is in place yet.
Track what runs today at [bento.tamnd.com](https://bento.tamnd.com) and in the [releases](https://github.com/tamnd/bento/releases).
If something you rely on is missing, open an issue.

## License

MIT. See [LICENSE](./LICENSE).
