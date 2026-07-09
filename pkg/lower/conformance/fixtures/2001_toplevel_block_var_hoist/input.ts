// JavaScript scopes a var to the whole function, so a var declared at the module top
// and redeclared inside a bare block at the top is one binding, not two. The value
// read after the block is the one the block assigned. Lowering the block var to a Go
// block-local would split the binding and leave the outer name undeclared at the read
// after the block, so the scope declares the var once at its top and the in-block var
// lowers to an assignment. The hoist walk has to step into a top statement that is
// itself a block, since starting one level below would step over the block and miss
// its var. test262 reaches this with the block-scope scope-var tests.
var x = "outside";
{
  var x = "inside";
}
console.log(x);
