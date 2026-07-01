package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/tamnd/bento/pkg/engine/quickjs"
)

// TestFSRoundTrip exercises the node fs module through the full runtime stack:
// write a file with the Go-backed host functions, read it back, and stat it.
func TestFSRoundTrip(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "note.txt")
	out, _ := run(t, `
		const fs = require("fs");
		const p = `+jsString(target)+`;
		fs.writeFileSync(p, "hello bento");
		console.log(fs.readFileSync(p, "utf8"));
		console.log(fs.existsSync(p));
		console.log(fs.statSync(p).size);
		console.log(fs.statSync(p).isFile());
	`)
	lines := strings.Fields(out)
	if !strings.Contains(out, "hello bento") {
		t.Errorf("readFileSync wrong: %q", out)
	}
	if len(lines) < 4 || lines[len(lines)-2] != "11" || lines[len(lines)-1] != "true" {
		t.Errorf("stat wrong: %q", out)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("file not actually written: %v", err)
	}
}

func TestFSReaddirAndMkdir(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "a", "b")
	out, _ := run(t, `
		const fs = require("fs");
		fs.mkdirSync(`+jsString(sub)+`, { recursive: true });
		fs.writeFileSync(`+jsString(filepath.Join(sub, "f.txt"))+`, "x");
		const names = fs.readdirSync(`+jsString(sub)+`);
		console.log(names.join(","));
	`)
	if !strings.Contains(out, "f.txt") {
		t.Errorf("readdir wrong: %q", out)
	}
}

func TestFSPromises(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "p.txt")
	out, _ := run(t, `
		const fs = require("fs").promises;
		(async () => {
			await fs.writeFile(`+jsString(target)+`, "async write");
			const data = await fs.readFile(`+jsString(target)+`, "utf8");
			console.log(data);
		})();
	`)
	if !strings.Contains(out, "async write") {
		t.Errorf("fs.promises round trip failed: %q", out)
	}
}

func TestFSErrorCode(t *testing.T) {
	out, _ := run(t, `
		const fs = require("fs");
		try {
			fs.readFileSync("/no/such/path/really-not-here");
		} catch (e) {
			console.log(e.code);
		}
	`)
	if !strings.Contains(out, "ENOENT") {
		t.Errorf("expected ENOENT, got: %q", out)
	}
}

func TestGlobalBuffer(t *testing.T) {
	out, _ := run(t, `console.log(Buffer.from("hi").toString("hex"));`)
	if !strings.Contains(out, "6869") {
		t.Errorf("global Buffer missing: %q", out)
	}
}

func TestPathThroughRuntime(t *testing.T) {
	out, _ := run(t, `
		import { join, basename } from "node:path";
		console.log(join("x", "y"));
		console.log(basename("/a/b.ts"));
	`)
	if !strings.Contains(out, "x/y") || !strings.Contains(out, "b.ts") {
		t.Errorf("path import failed: %q", out)
	}
}

// jsString renders a Go string as a JavaScript string literal for embedding in
// test programs, so Windows backslashes in temp paths survive intact.
func jsString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString("\\\\")
		case '"':
			b.WriteString("\\\"")
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
