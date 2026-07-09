// A var declared with no initializer and later assigned a function stores a boxed
// value.Value. TypeScript's control-flow analysis evolves the implicit any to the
// function type at the call site, but the slot itself stays a box, so the call must
// dispatch through the runtime Call rather than a static Go call the box does not
// support. The call result is a box too, so an enclosing String coercion routes
// through value.ToString rather than the number path the evolved return type would
// pick. test262 reaches this in the block-scope scope-var tests, where a closure
// stored in an implicit-any var is called after the block that assigned it.
var f;
f = function () { return 1; };
console.log(String(f()));
