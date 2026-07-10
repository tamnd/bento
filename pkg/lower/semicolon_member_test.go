package lower

import (
	"testing"
)

// TestSemicolonMemberSkipped pins that a stray semicolon in a class body is a
// no-op member, skipped rather than misread as a heritage clause, so a class
// sprinkled with them lowers and runs as if they were absent.
func TestSemicolonMemberSkipped(t *testing.T) {
	const src = `class Point {
  ;
  x: number;
  ;
  y: number;
  constructor(x: number, y: number) { this.x = x; this.y = y; }
  ;
  sum(): number { return this.x + this.y; }
}
console.log(String(new Point(3, 4).sum()));
`
	got := runProgramGo(t, src)
	if got != "7\n" {
		t.Errorf("semicolon members ran wrong\n got: %q\nwant: %q", got, "7\n")
	}
}
