// A function whose name is not a Go identifier mangles through the pure
// escape in ident.go: $ becomes D_, so $DONE declares and calls as D_DONE,
// and a local like $status reads the same way at every site. test262 spells
// this shape in every composed test ($DONOTEVALUATE, $DONE), so this is the
// single most common non-identifier name in practice.
function $DONE(msg: string): void {
  console.log("done: " + msg);
}

function $$twice(n: number): number {
  return n * 2;
}

$DONE("first");
$DONE("second");

let $status = $$twice(21);
console.log($status);
