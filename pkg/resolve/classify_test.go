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
		// A path rooted at a volume is as absolute as one rooted at a slash. These
		// reach the resolver when a loader hands back a path it already resolved,
		// and reading only the leading slash made them bare, so a Windows run went
		// looking for a package named C: in node_modules.
		{"C:/Users/x/app.mjs", classAbsolute},
		{"c:/users/x/app.mjs", classAbsolute},
		{"//server/share/x.js", classAbsolute},
		// A bare volume names the working directory on that drive rather than a
		// file, so it is not a path bento may hold and must not classify absolute.
		{"C:", classBare},
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
	// A Windows drive letter must not look like a URL scheme, and must not fall
	// through to bare either. Asserting only the first half is what let the bare
	// misclassification live: the shape was ruled out of the wrong bucket without
	// ever being put in the right one.
	if got, _ := classify("C:/x"); got != classAbsolute {
		t.Errorf("classify(C:/x) = %v, want absolute", got)
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
