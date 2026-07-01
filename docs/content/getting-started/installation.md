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

Next: [the quick start](/getting-started/quick-start/).
