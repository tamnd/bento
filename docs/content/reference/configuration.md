---
title: "Configuration"
description: "The package.json fields, environment variables, and module resolution bento reads."
weight: 20
---

bento is configured mostly through your project's `package.json` and command-line flags (see the [CLI reference](/reference/cli/)).
It reads a few environment variables for the Go toolchain and for cross-compilation.

## package.json

bento reads the same `package.json` your project already has:

- `main` (or the module entry) tells `bento run .` and `bento build .` where the program starts.
- `dependencies` and `devDependencies` drive `bento install`.
- `scripts` are runnable by name, so `bento run start` runs the `start` script.

You do not need a bento-specific config file to run an existing Node or Bun project.

## Environment variables

| Variable | Meaning |
|----------|---------|
| `GOOS` | Target operating system for `bento build`, passed to the Go toolchain (`linux`, `darwin`, `windows`, ...). |
| `GOARCH` | Target architecture for `bento build` (`amd64`, `arm64`, ...). |
| `GOTOOLCHAIN` | Go toolchain selection, honoured when bento invokes the compiler. |

Compile mode and `bento build` shell out to the Go compiler, so the standard Go environment applies to them.
`bento run` in run mode needs only bento itself.

## Module resolution

bento resolves imports from three places:

- **npm packages** resolve through `node_modules`, the way Node does. `bento install` populates it from `package.json`.
- **`node:` builtins** (`node:fs`, `node:path`, and the rest) resolve to bento's built-in implementations of the Node API.
- **`go:` imports** name a Go module path. bento resolves the Go module, generates the bridge, and makes its exported symbols callable from TypeScript. See [importing a Go library](/guides/importing-a-go-library/).

## Run mode and compile mode

bento picks how to execute each module rather than asking you to configure it.
A fully typed module can be lowered to Go and compiled for speed; a module that is not fully typed runs interpreted.
Behaviour is the same either way; the difference is how fast it runs.
Compile-mode coverage grows release to release, so a module that runs interpreted today may compile in a later version with no change on your side.
