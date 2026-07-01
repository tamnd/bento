package resolve

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		spec string
		want class
	}{
		{"zod", classBare},
		{"@scope/pkg", classBare},
		{"lodash/fp", classBare},
		{"react-dom/server", classBare},
		{"./util", classRelative},
		{"../lib/x", classRelative},
		{".", classRelative},
		{"..", classRelative},
		{"/abs/path", classAbsolute},
		{"#config", classImports},
		{"node:fs", classBuiltin},
		{"data:text/javascript,1", classData},
		{"go:github.com/x/y", classGo},
		{"file:///a/b", classAbsolute},
		{"http://example.com/x", classUnsupported},
		{"https://example.com/x", classUnsupported},
	}
	for _, c := range cases {
		got, _ := classify(c.spec)
		if got != c.want {
			t.Errorf("classify(%q) = %v, want %v", c.spec, got, c.want)
		}
	}
}

func TestClassifyDoesNotTouchDisk(t *testing.T) {
	// A slash inside a bare specifier must not make it relative.
	if got, _ := classify("lodash/fp"); got != classBare {
		t.Errorf("lodash/fp classified as %v, want bare", got)
	}
	// A Windows drive letter must not look like a URL scheme.
	if got, _ := classify("C:/x"); got == classUnsupported {
		t.Errorf("C:/x should not classify as an unsupported URL scheme")
	}
}

func TestFileURLToPath(t *testing.T) {
	cases := map[string]string{
		"//localhost/a/b": "/a/b",
		"///a/b":          "/a/b",
	}
	for in, want := range cases {
		if got := fileURLToPath(in); got != want {
			t.Errorf("fileURLToPath(%q) = %q, want %q", in, got, want)
		}
	}
}
