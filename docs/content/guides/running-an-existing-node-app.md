---
title: "Running an existing Node app"
description: "Bring a Node.js or Bun project onto bento without changing your code."
weight: 10
---

bento's compatibility goal is simple: if your project runs on Node.js or Bun, it should run on bento.
You do not port anything.
You point bento at the project and it reads the same `package.json`, resolves the same `node_modules`, and speaks the same APIs.

## Install dependencies

From the project root:

```bash
bento install
```

This reads `package.json` (and the lockfile if there is one) and populates `node_modules`.
Your existing lockfile is respected.

## Run the app

Run the project's entry point:

```bash
bento run .
```

bento uses the `main` field, or the module entry, the same way Node resolves it.
To run a specific file instead, name it:

```bash
bento run src/server.ts
```

## Run a package script

If your `package.json` defines scripts, run one by name:

```bash
bento run start
bento run test
```

## What works and what to expect

Run mode implements the Node.js and Bun APIs your code calls: the `node:` builtins, `process`, `fetch`, the file system, and the common surface a server or CLI reaches for.
Both TypeScript and JavaScript run directly, with no separate transpile step.

bento is early, so the API coverage has gaps.
If something your app depends on is missing, that is a coverage hole rather than a design limit, and the [release notes](/reference/release-notes/) track what has landed.
Fully typed modules may run through compile mode for extra speed, but that is automatic and does not change behaviour.
