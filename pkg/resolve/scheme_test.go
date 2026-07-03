package resolve

import (
	"testing"
)

func TestResolveDataJSON(t *testing.T) {
	r := newTestResolver(newMemFS(), true)
	got, err := r.Resolve(`data:application/json,{"a":1}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != KindData || got.Format != FormatJSON {
		t.Errorf("kind/format = %v/%v, want data/json", got.Kind, got.Format)
	}
	if string(got.Body) != `{"a":1}` {
		t.Errorf("body = %q, want the JSON payload", got.Body)
	}
}

func TestResolveDataBase64JS(t *testing.T) {
	// base64 of: export const x = 1
	r := newTestResolver(newMemFS(), true)
	got, err := r.Resolve("data:text/javascript;base64,ZXhwb3J0IGNvbnN0IHggPSAx", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Format != FormatESM {
		t.Errorf("format = %v, want esm", got.Format)
	}
	if string(got.Body) != "export const x = 1" {
		t.Errorf("body = %q, want decoded source", got.Body)
	}
}

func TestResolveDataUnsupportedMime(t *testing.T) {
	r := newTestResolver(newMemFS(), true)
	_, err := r.Resolve("data:image/png,xxxx", nil)
	if err == nil {
		t.Fatal("an unsupported data: mime should error")
	}
}

func TestResolveGo(t *testing.T) {
	r := newTestResolver(newMemFS(), true)
	got, err := r.Resolve("go:github.com/tamnd/bento/pkg/loop", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != KindGo || got.Path != "github.com/tamnd/bento/pkg/loop" {
		t.Errorf("unexpected go resolution: %+v", got)
	}
}

func TestResolveGoInvalid(t *testing.T) {
	r := newTestResolver(newMemFS(), true)
	// An empty path segment is a malformed import path. A bare standard-library
	// name like "io" is now valid, so the invalidity has to come from the path
	// shape rather than from a missing dotted host.
	_, err := r.Resolve("go:double//slash", nil)
	if err == nil {
		t.Fatal("an invalid go: path should error")
	}
}

func TestResolveGoStdlib(t *testing.T) {
	r := newTestResolver(newMemFS(), true)
	got, err := r.Resolve("go:crypto/sha256", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != KindGo || got.Path != "crypto/sha256" {
		t.Errorf("unexpected go stdlib resolution: %+v", got)
	}
}

func TestResolveGoPinnedVersion(t *testing.T) {
	r := newTestResolver(newMemFS(), true)
	got, err := r.Resolve("go:github.com/klauspost/compress/zstd@v1.17.9", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "github.com/klauspost/compress/zstd" {
		t.Errorf("path = %q, want the import path with the version split off", got.Path)
	}
	if got.GoVersion != "v1.17.9" {
		t.Errorf("GoVersion = %q, want v1.17.9", got.GoVersion)
	}
}
