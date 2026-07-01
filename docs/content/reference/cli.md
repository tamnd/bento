---
title: "CLI reference"
description: "Every bento command and flag."
weight: 10
---

```
bento [command] [flags]
```

The commands: `run` executes a file or project, `build` compiles it to a binary, `test` runs tests, `add`/`install`/`remove` manage dependencies, `x` runs a package binary, `repl` opens an interactive prompt, and `version` prints the build.
Run `bento <command> --help` for the canonical, up-to-date list.

## bento run

```
bento run <file|dir|script> [args...]
```

Runs a TypeScript or JavaScript file, a project directory, or a `package.json` script.
With a directory or `.`, bento resolves the project entry the way Node would.
Fully typed modules may run through compile mode; everything else runs in run mode.

```bash
bento run app.ts
bento run .
bento run start
bento run server.ts --port 3000
```

Arguments after the target are passed through to your program.

## bento build

```
bento build <file|dir> [flags]
```

Compiles a program to a single static, cgo-free executable.

| Flag | Default | Meaning |
|------|---------|---------|
| `-o, --out` | entry name | Output path for the binary |

```bash
bento build app.ts -o app
GOOS=linux GOARCH=amd64 bento build app.ts -o app-linux
```

Cross-compilation uses the standard `GOOS` and `GOARCH` environment variables, since build goes through the Go toolchain.

## bento test

```
bento test [pattern...]
```

Runs your test files.
With no pattern, bento discovers tests under the project; pass patterns to narrow the run.

```bash
bento test
bento test src/parser
```

## bento add, install, remove

Dependency management against `package.json`.

```
bento add <pkg>[@version]...
bento install [pkg...]
bento remove <pkg>...
```

- `bento add` adds packages to `package.json` and installs them.
- `bento install` with no arguments installs everything in `package.json`; with names, adds and installs those.
- `bento remove` drops packages from `package.json` and `node_modules`.

```bash
bento add zod
bento add typescript@5.5.0
bento install
bento remove left-pad
```

## bento x

```
bento x <pkg> [args...]
```

Runs a package's binary without installing it globally, the way `npx` does.

```bash
bento x cowsay hello
bento x prettier --write .
```

## bento repl

```
bento repl
```

Opens an interactive prompt.
Evaluate TypeScript expressions one line at a time, including `go:` imports.

```bash
bento repl
```

```
> const x = 40 + 2
> x
42
```

## bento version

```
bento version
```

Prints the bento version and build details.
