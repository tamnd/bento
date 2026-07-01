package frontend

import (
	"strings"
	"testing"
)

func TestTranspileStripsTypes(t *testing.T) {
	src := `interface P { name: string }
const p: P = { name: "bento" };
export function hi(x: number): number { return x * 2 }
`
	res, err := Transpile(src, Options{Filename: "x.ts"})
	if err != nil {
		t.Fatalf("transpile: %v", err)
	}
	if strings.Contains(res.Code, "interface") {
		t.Errorf("interface should be stripped, got:\n%s", res.Code)
	}
	if !strings.Contains(res.Code, "name") {
		t.Errorf("value code should survive, got:\n%s", res.Code)
	}
}

func TestTranspileCommonJS(t *testing.T) {
	// ESM import should lower to a require call in CommonJS output.
	res, err := Transpile(`import { join } from "node:path"; join("a", "b");`, Options{Filename: "m.ts"})
	if err != nil {
		t.Fatalf("transpile: %v", err)
	}
	if !strings.Contains(res.Code, "require") {
		t.Errorf("expected require in CommonJS output, got:\n%s", res.Code)
	}
}

func TestTranspileSyntaxError(t *testing.T) {
	_, err := Transpile(`const = ;`, Options{Filename: "bad.ts"})
	if err == nil {
		t.Fatal("expected a syntax error")
	}
	if !strings.Contains(err.Error(), "bad.ts") {
		t.Errorf("error should name the file, got: %v", err)
	}
}

func TestLoaderFor(t *testing.T) {
	cases := map[string]bool{"a.ts": true, "a.tsx": true, "a.jsx": true, "a.js": true, "a.mjs": true}
	for name := range cases {
		if _, err := Transpile(`const x = 1;`, Options{Filename: name}); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
