---
title: "Installation"
description: "Install bento from Go, Homebrew, Scoop, a release archive, a Linux package, or the container image."
weight: 20
---

bento is a single static binary with no cgo, so there is nothing to link and nothing else to install.
Pick whichever channel suits you.

## Go

```bash
go install github.com/tamnd/bento/cmd/bento@latest
```

## Homebrew (macOS and Linux)

```bash
brew install tamnd/tap/bento
```

## Scoop (Windows)

```bash
scoop bucket add tamnd https://github.com/tamnd/scoop-bucket
scoop install bento
```

## Linux (apt and dnf)

A signed apt and dnf repository tracks every release, so `apt upgrade` and `dnf upgrade` keep bento current.

```bash
# Debian, Ubuntu
curl -fsSL https://tamnd.github.io/linux-repo/gpg.key \
  | sudo gpg --dearmor -o /usr/share/keyrings/tamnd.gpg
echo "deb [signed-by=/usr/share/keyrings/tamnd.gpg] https://tamnd.github.io/linux-repo/apt stable main" \
  | sudo tee /etc/apt/sources.list.d/tamnd.list
sudo apt update && sudo apt install bento

# Fedora, RHEL
sudo dnf config-manager --add-repo https://tamnd.github.io/linux-repo/dnf/tamnd.repo
sudo dnf install bento
```

## Release archives and Linux packages

Every [release](https://github.com/tamnd/bento/releases) attaches `tar.gz` archives (and a `.zip` for Windows) for Linux, macOS, Windows, and FreeBSD, plus `.deb`, `.rpm`, and `.apk` packages and a `checksums.txt` with a cosign signature.
The archives cover eight targets: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`, `windows/arm64`, `freebsd/amd64` and `freebsd/arm64`.
Download the one for your platform, extract `bento`, and put it on your `PATH`.
To install a package directly without the repo above:

```bash
# Debian, Ubuntu
sudo dpkg -i bento_*_amd64.deb

# Fedora, RHEL
sudo rpm -i bento-*.x86_64.rpm
```

## Container

```bash
docker run -v "$PWD:/app" -w /app ghcr.io/tamnd/bento run app.ts
```

## Compiling to a binary needs a Go toolchain

Running TypeScript with `bento run` needs only bento itself.
Compile mode and `bento build` lower code to Go and invoke the Go compiler, so those paths need a Go toolchain on your `PATH`.
Check with `go version`; install Go from [go.dev](https://go.dev/dl/) if it is missing.

They also need a checkout of the bento source, which an installed binary does not carry.
The generated Go imports bento's runtime packages, and bento looks for the module by walking up from its own location and from your working directory until it finds the `go.mod` that declares `github.com/tamnd/bento`.
Without one it stops with `cannot locate the bento module source`.
So either work inside a clone:

```bash
git clone https://github.com/tamnd/bento
cd bento
```

or point bento at one from anywhere:

```bash
export BENTO_MODULE_ROOT=/path/to/bento
bento build app.ts
```

This is a rough edge rather than a design, tracked in [#765](https://github.com/tamnd/bento/issues/765).

## Platform support

Linux, macOS, and Windows are all first-class.
The test suite runs on all three in CI, and Linux and Windows also run the ahead-of-time equivalence suite, which compiles each fixture to a real binary and executes it.
Every pull request unpacks the shipped Windows zip and the Linux tarball, runs the binary inside, and compiles a program with it, so an archive that would not work does not get published.

There is no 32-bit build.
A JavaScript array index runs to 2^53 - 1 and the value runtime keeps that bound in an `int`, which does not fit in 32 bits.
A wider constant would not fix it, since the same number has to index a Go slice.

Next: [the quick start](/getting-started/quick-start/).
