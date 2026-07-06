// A thrown string wraps in the runtime's ThrownString: uncaught, it reports
// the string the way node reports a thrown primitive and the process exits
// non-zero, which is exactly what test262's $DONOTEVALUATE needs from a
// statement that must never run.
function refuse(): never {
  throw "refused: this statement must not run";
}

console.log("before");
refuse();
console.log("unreachable");
