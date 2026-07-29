package lower

import (
	"strings"
	"testing"
)

// A + over an operand the lowerer boxed produces a value.Value, whatever the checker
// made of the sum. Two places used to decide otherwise and each broke a different
// way: the assignment bridge read only the checker's type and dropped the box into a
// float64 slot, which failed at the Go compiler with the reason lost, and the
// operator table read only the checker's type of the inner sum and handed the second
// term of a chain back.
//
// The shape is ordinary. `let total = 0; total = total + os.totalmem()` and
// `busy += c.times.user + c.times.sys` are how a program adds up what a built-in
// answered, and neither compiled.

// TestDynamicAddIntoAStaticSlotCoerces pins the assignment bridge: the box coerces
// down through the ToNumber family into whatever static primitive slot it flows
// into, rather than landing in the slot as a value.Value the Go compiler rejects.
// The declaration, the assignment, and the return all reach the same bridge, so all
// three are checked.
func TestDynamicAddIntoAStaticSlotCoerces(t *testing.T) {
	for name, src := range map[string]string{
		"assignment":  "const os = require('os');\nlet n = 0;\nn = n + os.totalmem();\nconsole.log(n);\n",
		"declaration": "const os = require('os');\nconst n: number = 1 + os.totalmem();\nconsole.log(n);\n",
		"return":      "const os = require('os');\nfunction f(): number { return 1 + os.totalmem(); }\nconsole.log(f());\n",
	} {
		t.Run(name, func(t *testing.T) {
			got := renderProgram(t, src)
			if !strings.Contains(got, "value.ToNumber(value.Add(") {
				t.Errorf("%s emitted:\n%s\nwant the boxed sum coerced through value.ToNumber", name, got)
			}
		})
	}
}

// TestDynamicAddChainsThroughItsOwnResult pins the operator table: the result of a
// boxed + is itself boxed, so a second + over it takes the boxed path too. Before
// this the chain handed back at its second term with "binary operator on mixed or
// non-primitive operands", because the checker types the inner sum a number and the
// table asked only that.
func TestDynamicAddChainsThroughItsOwnResult(t *testing.T) {
	got := renderProgram(t, "const os = require('os');\n"+
		"const c = os.cpus()[0].times;\n"+
		"const n: number = c.user + c.sys + c.idle;\n"+
		"console.log(n);\n")
	if strings.Count(got, "value.Add(") != 2 {
		t.Errorf("a three-term sum over boxed reads emitted:\n%s\nwant two nested value.Add calls", got)
	}
}

// TestDynamicAddInACompoundAssignment pins the compound form, which reaches the
// coercion by its own path in stmt.go rather than through the assignment bridge.
// `busy += a + b` over boxed reads is the shape a program that sums a core's states
// is written in.
func TestDynamicAddInACompoundAssignment(t *testing.T) {
	got := renderProgram(t, "const os = require('os');\n"+
		"const c = os.cpus()[0].times;\n"+
		"let busy = 0;\n"+
		"busy += c.user + c.sys;\n"+
		"console.log(busy);\n")
	if !strings.Contains(got, "value.ToNumber(value.Add(") {
		t.Errorf("busy += c.user + c.sys emitted:\n%s\nwant the boxed sum coerced back into the number slot", got)
	}
}

// TestDynamicArithmeticIsNotJustPlus pins that the widening is generic. Neither of
// the two places changed reads the operator, so `-`, `*` and the rest of the numeric
// family reach the value model over a boxed operand the same way `+` does, and each
// coerces back into the number slot it flows into. `used = total - free` is the shape,
// and it is as common as the sum.
func TestDynamicArithmeticIsNotJustPlus(t *testing.T) {
	for _, op := range []string{"-", "*", "/", "%"} {
		t.Run(op, func(t *testing.T) {
			got := renderProgram(t, "const os = require('os');\n"+
				"const n: number = os.totalmem() "+op+" os.freemem();\n"+
				"console.log(n);\n")
			if !strings.Contains(got, "value.ToNumber(") {
				t.Errorf("a %s over two boxed reads emitted:\n%s\nwant the result coerced back into the number slot", op, got)
			}
		})
	}
}

// TestStaticAddKeepsTheGoOperator pins the guard. The widening keys off a lowering
// that is a box, so a sum of two ordinary numbers must be untouched: it keeps Go's
// own + and never reaches the value model, which is the whole reason the static
// paths exist.
func TestStaticAddKeepsTheGoOperator(t *testing.T) {
	got := renderProgram(t, "let a = 1;\nlet b = 2;\nconsole.log(a + b);\n")
	if strings.Contains(got, "value.Add(") {
		t.Errorf("a + b over two numbers emitted:\n%s\nwant Go's own operator", got)
	}
}

// TestStringAddOverABoxKeepsConcatenating pins the other guard, the one that was
// already there: a + with a known string operand always concatenates, so its result
// kind is known after all and it stays off the boxed path with the dynamic operand
// running through ToString. Widening what counts as a boxed operand must not have
// pulled this case in.
func TestStringAddOverABoxKeepsConcatenating(t *testing.T) {
	got := renderProgram(t, "const os = require('os');\n"+
		"const s: string = 'cores: ' + os.availableParallelism();\n"+
		"console.log(s);\n")
	if strings.Contains(got, "value.Add(") {
		t.Errorf("'cores: ' + os.availableParallelism() emitted:\n%s\nwant a string concat, not a boxed add", got)
	}
}
