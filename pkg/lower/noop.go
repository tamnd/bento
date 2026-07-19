package lower

import (
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the two statements that carry no runtime effect, the empty
// statement `;` and the debugger statement `debugger;`. The frontend leaves both
// unnamed, so each surfaces as an unclassified node the lowering recognizes by its
// text. An empty statement is a lone semicolon a program writes where a statement
// is grammatical but none is wanted, and it does nothing. A debugger statement
// pauses execution when a debugger is attached and is a no-op otherwise; an AOT
// binary has no attached debugger, so dropping it is the faithful runtime behavior.
// Both lower to nothing at all, which is the readable Go for a statement that does
// nothing, so the caller drops them rather than emitting an empty Go statement.

// isNoopStatement reports whether n is an empty or debugger statement. Both are
// unclassified nodes with no children whose text is the whole statement, so the
// classification is by trimmed text alone.
func (r *Renderer) isNoopStatement(n frontend.Node) bool {
	if n.Kind() != frontend.NodeUnknown || len(r.prog.Children(n)) != 0 {
		return false
	}
	switch strings.TrimSpace(r.prog.Text(n)) {
	case ";", "debugger", "debugger;":
		return true
	}
	return false
}
