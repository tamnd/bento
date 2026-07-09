// A local declared with no initializer and then only written, never read, is
// declared-and-not-used in the emitted Go even though JavaScript names it more than
// once: Go counts only a read toward use, and a plain `x = e` is a write. The
// lowerer marks such a binding used with the same trailing blank it gives an unread
// `var x = e`, so the initializer's slot still exists and the program compiles.
// test262's asi suite reaches this with `var x \n x = 1`.
var x;
x = 1;
console.log("ok");
