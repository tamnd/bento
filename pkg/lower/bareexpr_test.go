package lower

import (
	"strings"
	"testing"
)

// TestBareExpressionStatementDiscards pins that an expression statement that is
// not an assignment, update, or call lowers to the _ = expr discard: Go has no
// bare-value statement, so the operand is evaluated and its result dropped. A
// lone identifier and a discarded comparison both take the same form.
func TestBareExpressionStatementDiscards(t *testing.T) {
	src := "let x: number = 5;\nx;\nx < 10;\nconsole.log(x);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "_ = x\n") {
		t.Errorf("a lone identifier statement did not lower to a discard:\n%s", source)
	}
	if !strings.Contains(source, "_ = x < 10") {
		t.Errorf("a discarded comparison did not lower to a discard:\n%s", source)
	}
}

// TestBareExpressionStatementRuns builds and runs a program whose bare statements
// have no effect, checking the discards compile and the program still prints the
// one value it computes.
func TestBareExpressionStatementRuns(t *testing.T) {
	skipIfShort(t)
	const src = "let x: number = 5;\nx;\nx < 10;\nx + 1;\nconsole.log(x);\n"
	got := runProgramGo(t, src)
	if got != "5\n" {
		t.Fatalf("bare-statement program printed %q, want %q", got, "5\n")
	}
}
