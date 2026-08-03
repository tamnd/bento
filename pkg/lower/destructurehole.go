package lower

import (
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// An array pattern can skip a position by writing nothing between two commas, so
// `const [, second] = arr` binds one name off the second slot and `const [a, , c] = arr`
// binds the first and the third. The skipped position is a hole: it names nothing, reads
// nothing, and exists only to hold the place, which is what makes the elements after it
// select the indices they do.
//
// Every array pattern classified its elements one at a time and refused an element that
// held no binding, so a hole anywhere in a pattern handed the whole unit back:
//
//	an array destructuring hole or rest is a later slice
//
// The frontend keeps a hole as an element of its own, an empty node with no children
// sitting in the position it skips, so the positions are already right. What was missing
// was the classification saying so and each caller stepping over it.

// arrayHoleElem reports whether an array pattern element is a hole, the elision between
// two commas. The frontend gives a hole an element of its own so the elements after it
// keep their positions, and that element is empty: no text and no children, unlike a
// binding element, which wraps at least its identifier. A trailing comma, `[a, ]`, is not
// an elision and produces no such element, so it is not a hole and never was.
func (r *Renderer) arrayHoleElem(el frontend.Node) bool {
	if el.Kind() != frontend.NodeUnknown {
		return false
	}
	if strings.TrimSpace(r.prog.Text(el)) != "" {
		return false
	}
	return len(r.prog.Children(el)) == 0
}
