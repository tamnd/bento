---
title: "bento"
description: "bento is a TypeScript runtime built in Go, a Bun alternative. Run your existing Node.js and Bun code unchanged, compile typed TypeScript to Go for speed, and import any Go library from TypeScript, all from one pure-Go static binary with zero cgo."
heroTitle: "TypeScript, running on Go"
heroLead: "bento runs the TypeScript and JavaScript you already have, then goes further: type-checked code compiles down to Go for native speed, and any Go package is importable from TypeScript with a plain import. One static binary, pure Go, no cgo."
heroPrimaryURL: "/getting-started/quick-start/"
heroPrimaryText: "Get started"
---

Most TypeScript runtimes hand your code to a JavaScript engine and stop there.
bento starts the same way, so your existing Node.js and Bun projects run without a rewrite, but it treats typed TypeScript as something it can compile.
Code that is fully typed is lowered to Go and built into the binary, so the hot paths run as native Go instead of interpreted JavaScript.

Run a file, or a whole project, with one command:

```bash
bento run app.ts
bento run .
```

## What it does

- **Runs your existing code.** Node.js and Bun projects run unchanged. bento reads `package.json`, resolves `node_modules`, and speaks the Node and Bun APIs your code already calls.
- **Compiles typed TypeScript to Go.** Where the types are complete, bento lowers TypeScript to Go and compiles it, so those paths run at Go speed rather than through an interpreter.
- **Imports Go libraries from TypeScript.** `import { NewReader } from "go:github.com/klauspost/compress/zstd"` pulls a Go package straight into your TypeScript, no bindings to hand-write.
- **Ships as one binary.** bento is pure Go with no cgo, so `bento build` produces a single static executable you can drop on a server or into a container.

## Honest status

bento is early.
The runtime and the Go import bridge work for real programs, but the surface is not complete and things will change between releases.
The [release notes](/reference/release-notes/) track what actually landed in each version.

## Where to go next

- New here? Read the [introduction](/getting-started/introduction/), then the [quick start](/getting-started/quick-start/).
- Want to install it? See [installation](/getting-started/installation/).
- Have a specific job? The [guides](/guides/) cover running an existing Node app, importing a Go library, and building a single binary.
- Need every command? The [CLI reference](/reference/cli/) is the full surface.
