// Object.keys and its siblings read only the receiver's compile-time shape, so the
// fold builds the name list from the field names and never lowers the argument. When
// the receiver's only read is that call, the argument would be orphaned and the Go
// build would reject the declared-and-not-used binding. The fold records the dropped
// read so the binding is blanked with _ = o, the same way an unused var is, and the
// emitted Go builds and runs. A receiver read elsewhere stays used and is not blanked.
var o = { x: 1, y: 2 };
var a = Object.keys(o);
console.log(String(a.length));
console.log(a[0]);
console.log(a[1]);

var p = { a: 10, b: 20 };
var b = Object.hasOwn(p, "a");
console.log(String(b));

var q = { m: 1, n: 2 };
var names = Object.getOwnPropertyNames(q);
console.log(String(q.m));
console.log(String(names.length));
