package lower

import (
	"strings"
	"testing"
)

// JavaScript lets a break name a labeled block, `label: { ...; break label; ... }`,
// jumping past the block. Go has no label on a plain block, so the block lowers to a
// one-shot for loop the label sits on: break label targets the loop the way it
// targeted the block, and a trailing bare break runs the body exactly once.

// TestLabeledBlockBreakLowersToOneShotLoop proves a labeled block a break targets
// lowers to a labeled for loop carrying a trailing bare break.
func TestLabeledBlockBreakLowersToOneShotLoop(t *testing.T) {
	const src = "let out = 0; block: { out = 1; break block; out = 2; } console.log(out);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "block:\n\tfor {") {
		t.Errorf("labeled block did not lower to a labeled for loop:\n%s", source)
	}
	if !strings.Contains(source, "break block") {
		t.Errorf("labeled break did not keep its target:\n%s", source)
	}
}

// TestLabeledBlockBreakRuns builds and runs a labeled block broken early, a block
// whose break sits inside a nested loop, and a nested loop whose own bare break
// stays in that inner loop, so the target of each break is proven against the
// JavaScript result rather than just the emitted shape.
func TestLabeledBlockBreakRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
let out = 0;
block: {
  if (out === 0) {
    out = 1;
    break block;
  }
  out = 2;
}
console.log(out);

let found = -1;
search: {
  for (let i = 0; i < 3; i++) {
    if (i === 2) {
      found = i;
      break search;
    }
  }
  found = 99;
}
console.log(found);

const log: number[] = [];
blk: {
  for (let i = 0; i < 3; i++) {
    if (i === 1) break;
    log.push(i);
  }
  log.push(9);
  break blk;
  log.push(100);
}
console.log(log.join(","));
`
	if got, want := runProgramGo(t, src), "1\n2\n0,9\n"; got != want {
		t.Fatalf("labeled block break printed %q, want %q", got, want)
	}
}
