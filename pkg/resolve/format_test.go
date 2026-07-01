package resolve

import "testing"

func TestDetectFormatByExtension(t *testing.T) {
	r := newTestResolver(newMemFS(), true)
	cases := map[string]Format{
		"/a/x.mjs":  FormatESM,
		"/a/x.mts":  FormatESM,
		"/a/x.cjs":  FormatCommonJS,
		"/a/x.cts":  FormatCommonJS,
		"/a/x.json": FormatJSON,
		"/a/x.node": FormatCommonJS,
	}
	for path, want := range cases {
		if got := r.detectFormat(path); got != want {
			t.Errorf("detectFormat(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestDetectFormatAmbiguousDefaults(t *testing.T) {
	// A bare .ts with no governing package.json defaults to ESM (DX divergence),
	// a bare .js defaults to CommonJS like Node.
	r := newTestResolver(newMemFS(), true)
	if got := r.detectFormat("/nowhere/a.ts"); got != FormatESM {
		t.Errorf("bare .ts = %v, want esm", got)
	}
	if got := r.detectFormat("/nowhere/a.js"); got != FormatCommonJS {
		t.Errorf("bare .js = %v, want commonjs", got)
	}
}

func TestDetectFormatFollowsPackageType(t *testing.T) {
	fs := newMemFS().
		add("/app/package.json", `{"type":"module"}`).
		add("/app/a.js", "").
		add("/legacy/package.json", `{"type":"commonjs"}`).
		add("/legacy/a.ts", "")
	r := newTestResolver(fs, true)

	if got := r.detectFormat("/app/a.js"); got != FormatESM {
		t.Errorf("/app/a.js under type:module = %v, want esm", got)
	}
	if got := r.detectFormat("/legacy/a.ts"); got != FormatCommonJS {
		t.Errorf("/legacy/a.ts under type:commonjs = %v, want commonjs", got)
	}
}

func TestFormatOnResolvedModule(t *testing.T) {
	fs := newMemFS().
		add("/app/package.json", `{"type":"module"}`).
		add("/app/index.js", "").
		add("/app/dep.js", "")
	r := newTestResolver(fs, true)

	got, err := r.Resolve("./dep.js", parentESM("/app/index.js"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Format != FormatESM {
		t.Errorf("format = %v, want esm from the package type", got.Format)
	}
}
