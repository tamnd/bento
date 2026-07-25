package lower

import "testing"

// Object.defineProperty with a symbol as the descriptor throws a TypeError: a
// symbol is not an object, so ToPropertyDescriptor rejects it. The descriptor
// validation builds its message without abstract-ToString-ing the symbol, so the
// rejection is a clean throw rather than the infinite ToString recursion a symbol
// descriptor used to trigger.
func TestDefinePropertySymbolDescriptorThrows(t *testing.T) {
	skipIfShort(t)
	src := "var threw = false;\ntry {\nObject.defineProperty({}, 'a', Symbol());\n} catch (e) {\nthrew = e instanceof TypeError;\n}\nconsole.log(String(threw));\n"
	if got := runProgramGoTolerant(t, src); got != "true\n" {
		t.Fatalf("defineProperty symbol descriptor: got %q, want true (TypeError)", got)
	}
}

// A bare String coercion of a symbol through the abstract ToString throws a
// TypeError, the spec's rule that a symbol has no string form. The String built-in
// and Symbol.prototype.toString still render "Symbol(desc)"; only the abstract
// coercion (here the implicit one a template or concatenation would reach) throws.
func TestSymbolAbstractToStringThrows(t *testing.T) {
	skipIfShort(t)
	src := "var threw = false;\nvar s: any = Symbol('x');\ntry {\nvar out: any = '' + s;\nconsole.log(String(out));\n} catch (e) {\nthrew = e instanceof TypeError;\n}\nconsole.log(String(threw));\n"
	if got := runProgramGoTolerant(t, src); got != "true\n" {
		t.Fatalf("abstract ToString of symbol: got %q, want true (TypeError)", got)
	}
}

// The String built-in renders a symbol as its descriptive string rather than
// throwing, the SymbolDescriptiveString path that stays distinct from the abstract
// ToString the concatenation case throws on.
func TestStringBuiltinOfSymbolRenders(t *testing.T) {
	skipIfShort(t)
	src := "var s: any = Symbol('x');\nconsole.log(String(s));\n"
	if got := runProgramGoTolerant(t, src); got != "Symbol(x)\n" {
		t.Fatalf("String(symbol): got %q, want Symbol(x)", got)
	}
}
