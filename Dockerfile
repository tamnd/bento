# Consumed by GoReleaser: it copies the already cross-compiled binary out of the
# build context rather than compiling, so the image build is fast and uses the
# same static binary every other artifact ships.
#
# This image is tiny on purpose. bento is a pure-Go static binary with the JS
# engine (a pure-Go ES2023 engine) compiled straight into it, so there is no
# runtime to install: no Node, no V8, no shared libraries. All the image adds is
# ca-certificates for HTTPS and tzdata for sane timestamps.
#
# GoReleaser builds one multi-platform image with buildx and stages each
# platform's binary under a $TARGETPLATFORM directory (e.g. linux/amd64/) in the
# build context, so the COPY line selects the right one through the automatic
# TARGETPLATFORM build arg.
FROM alpine:3.21

ARG TARGETPLATFORM

RUN apk add --no-cache ca-certificates tzdata

COPY $TARGETPLATFORM/bento /usr/bin/bento

WORKDIR /work

# Mount your project and run a script against it:
#
#   docker run -v "$PWD:/work" ghcr.io/tamnd/bento run app.ts
ENTRYPOINT ["/usr/bin/bento"]
