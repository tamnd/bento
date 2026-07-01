---
title: "Building a single binary"
description: "Compile a bento program to one static, cgo-free executable you can ship anywhere."
weight: 30
---

Because bento is pure Go with no cgo, it can compile your program to a single static binary.
There is no runtime to install next to it and nothing to link at load time, so shipping a bento app is copying one file.

## Build a program

Point `bento build` at your entry file:

```bash
bento build app.ts -o app
```

The output `app` is a self-contained executable:

```bash
./app
```

Leave `-o` off and bento names the binary after the entry.

## What build does

`bento build` compiles as much of your program as it can through compile mode, lowering typed TypeScript to Go, and packages the rest so the binary is complete on its own.
Any `go:` imports are compiled in directly, since they are Go already.
The result is a normal Go-built static binary: no shared libraries, no interpreter shipped alongside it.

## Cross-compiling

Building goes through the Go toolchain, so cross-compilation uses the familiar `GOOS` and `GOARCH`:

```bash
GOOS=linux GOARCH=amd64 bento build app.ts -o app-linux
GOOS=darwin GOARCH=arm64 bento build app.ts -o app-macos
```

## Ship it in a container

A static binary drops into a tiny image with nothing else in it:

```dockerfile
FROM scratch
COPY app /app
ENTRYPOINT ["/app"]
```

## Status

`bento build` compiles what compile mode covers today, and that coverage is growing.
Programs that lean on parts of the API bento does not yet implement will surface that at build time.
The [release notes](/reference/release-notes/) track what each version can build.
