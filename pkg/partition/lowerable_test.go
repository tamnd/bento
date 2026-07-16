package partition

import (
	"testing"

	"github.com/tamnd/bento/pkg/frontend"
)

// firstFuncParamType walks a loaded program to its first function declaration and
// returns the declared type of that function's first parameter, the seam a test
// uses to hand lowerable a concrete type off a real checked program.
func firstFuncParamType(t *testing.T, p *frontend.Program) frontend.Type {
	t.Helper()
	var walk func(nodes []frontend.Node) (frontend.Node, bool)
	walk = func(nodes []frontend.Node) (frontend.Node, bool) {
		for _, n := range nodes {
			if n.Kind() == frontend.NodeFunctionDeclaration {
				return n, true
			}
			if f, ok := walk(p.Children(n)); ok {
				return f, ok
			}
		}
		return nil, false
	}
	fn, ok := walk(p.SourceFiles())
	if !ok {
		t.Fatal("no function declaration found")
	}
	sig, ok := p.SignatureAt(fn)
	if !ok || len(sig.Params) == 0 {
		t.Fatal("function has no first parameter")
	}
	return sig.Params[0].Type
}

// TestLowerableTuple proves the partitioner keeps a tuple on the compiled path
// when its elements lower and demotes it when an element does not. A tuple
// answers TupleElements, not ElementType, so lowerable judges it on its
// positional element types rather than on the inherited array members its object
// properties would report (typed/05 delivery slice 1).
func TestLowerableTuple(t *testing.T) {
	t.Run("all elements lowerable", func(t *testing.T) {
		p := loadReal(t, "export function f(pair: [string, number]): number { return pair.length; }\n", false)
		if !lowerable(p, firstFuncParamType(t, p)) {
			t.Error("a [string, number] tuple is not lowerable; want lowerable")
		}
	})
	t.Run("an any element demotes the tuple", func(t *testing.T) {
		p := loadReal(t, "export function f(pair: [string, any]): number { return pair.length; }\n", false)
		if lowerable(p, firstFuncParamType(t, p)) {
			t.Error("a tuple with an any element is lowerable; want demotion")
		}
	})
}
