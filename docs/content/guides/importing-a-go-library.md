---
title: "Importing a Go library"
description: "Call any Go package from TypeScript with a go: import, no bindings to write."
weight: 20
---

bento lets TypeScript reach straight into the Go ecosystem.
A `go:` import names a Go package by its module path, and bento generates the bridge for you.
The Go standard library and any public Go module are both fair game.

## A go: import

The import specifier is `go:` followed by the Go package path:

```ts
import { NewReader } from "go:github.com/klauspost/compress/zstd";
```

bento resolves the module, generates the bridge, and the exported Go symbols become callable values in your TypeScript, with types inferred from the Go signatures.

## A worked example

Decompress a zstd file using the Go `zstd` package directly:

```ts
import { NewReader } from "go:github.com/klauspost/compress/zstd";
import { readFileSync, writeFileSync } from "node:fs";

const input = readFileSync("payload.zst");
const reader = NewReader(input);
const out = reader.readAll();
writeFileSync("payload.out", out);

console.log(`decompressed ${input.length} -> ${out.length} bytes`);
```

Run it:

```bash
bento run decompress.ts
```

## The standard library too

The Go standard library is available under its normal import paths:

```ts
import { Sum256 } from "go:crypto/sha256";

const digest = Sum256(new TextEncoder().encode("hello"));
console.log(digest);
```

## How the mapping works

bento maps Go signatures onto TypeScript values.
Exported Go functions become callable, exported types become usable, and a Go function that returns a value and an error surfaces the error the way you would expect in TypeScript rather than as a silent second return.
Types are carried across from the Go side, so calls are checked.

This is one of the newest parts of bento.
The bridge works for real packages, but not every Go type shape maps cleanly yet, and the edges are still moving.
If a package does not bridge the way you expect, check the [release notes](/reference/release-notes/) for the current state.
