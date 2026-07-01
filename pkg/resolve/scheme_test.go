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
	// No dotted host, so it is not a valid go import path.
	_, err := r.Resolve("go:notahost/x", nil)
	if err == nil {
		t.Fatal("an invalid go: path should error")
	}
}

func TestValidGoImportPath(t *testing.T) {
	cases := map[string]bool{
		"github.com/x/y":    true,
		"golang.org/x/net":  true,
		"notahost/x":        false,
		"":                  false,
		"github.com":        false,
		"github.com//empty": false,
	}
	for in, want := range cases {
		if got := validGoImportPath(in); got != want {
			t.Errorf("validGoImportPath(%q) = %v, want %v", in, got, want)
		}
	}
}
