---
title: "Introduction"
description: "How bento runs TypeScript, when it compiles to Go, and what the Go import bridge buys you."
weight: 10
---

TypeScript today usually means one of two things: transpile to JavaScript and run it on a JavaScript engine, or run it on a runtime like Bun that bundles the engine for you.
Both are interpret-first.
bento keeps that path for compatibility, then adds a second one: typed code it can compile.

Say you have `app.ts`.
Run it and it just works:

```bash
bento run app.ts
```

Under that single command bento decides how to execute your code.

## Run mode

By default bento runs your TypeScript and JavaScript the way you expect.
It reads `package.json`, resolves `node_modules`, and implements the Node.js and Bun APIs, so an existing project runs without changes.
This is the compatibility floor: if your code runs on Node or Bun, the goal is that it runs on bento.

## Compile mode

When a module is fully typed, bento can lower it to Go and compile it into the binary rather than interpreting it.
The types carry enough information to pick real Go types and calls, so a typed hot path runs as native Go.
You do not rewrite anything.
The same TypeScript source is the input; bento chooses the faster path where the types allow it.

Compile mode is where the speed comes from, and it is also the newest part of bento, so its coverage grows release to release.
Code that is not fully typed still runs, just in run mode.

## The Go bridge

The third thing bento does is let TypeScript reach into the Go ecosystem directly.
A `go:` import names a Go package by its module path, and bento wires it up:

```ts
import { NewReader } from "go:github.com/klauspost/compress/zstd";

const r = NewReader(input);
```

There are no hand-written bindings and no separate build step for the glue.
bento resolves the Go module, generates the bridge, and the Go symbols are callable from TypeScript with types inferred from the Go signatures.
That means the whole Go standard library and any Go module on the internet is available to your TypeScript program.

## One binary, pure Go, no cgo

bento is written in Go and uses no cgo, so it builds to a single static executable with nothing to link against at runtime.
`bento build` produces that binary for your program too, which is what makes shipping a bento app a matter of copying one file.

## Honest status

bento is early.
Run mode handles real Node and Bun programs, and the Go bridge works for real Go packages, but neither surface is finished.
Compile mode covers a growing subset of typed TypeScript rather than all of it.
Expect gaps and expect the API to move between versions.
The [release notes](/reference/release-notes/) are the honest record of what each version actually does.

Next: [install bento](/getting-started/installation/).
