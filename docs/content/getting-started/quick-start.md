---
title: "Quick start"
description: "Run a TypeScript file, run an existing Node project unchanged, and import a Go library, all with bento."
weight: 30
---

This walks the core loop: run a single file, run a whole project the way Node would, and reach into Go from TypeScript.

## 1. Run a file

Save this as `app.ts`:

```ts
const name = "bento";
console.log(`hello from ${name}`);
```

Run it:

```bash
bento run app.ts
```

```
hello from bento
```

No config, no build step.
bento reads the file, runs it, and prints the output.

## 2. Run an existing project

bento aims to run Node.js and Bun code unchanged.
Point it at a project with a `package.json` and it resolves dependencies and runs the entry the same way Node would:

```bash
cd my-node-app
bento install     # populate node_modules
bento run .       # run the "main" / entry from package.json
```

If your project defines scripts, run one directly:

```bash
bento run start
```

## 3. Import a Go library

A `go:` import pulls a Go package straight into TypeScript.
Save this as `zstd.ts`:

```ts
import { NewReader } from "go:github.com/klauspost/compress/zstd";
import { readFileSync, writeFileSync } from "node:fs";

const compressed = readFileSync("data.zst");
const r = NewReader(compressed);
writeFileSync("data.out", r.readAll());
```

Run it:

```bash
bento run zstd.ts
```

bento resolves the Go module, generates the bridge, and calls into `zstd` directly.
The whole Go ecosystem is available this way, no bindings to write.

## 4. Build a single binary

When you are ready to ship, compile the program to one static executable:

```bash
bento build app.ts -o app
./app
```

The output is a self-contained binary with no runtime to install alongside it.

## Where to go next

- The [guides](/guides/) walk through running an existing Node app, importing a Go library, and building a binary in depth.
- The [CLI reference](/reference/cli/) lists every command and flag.
