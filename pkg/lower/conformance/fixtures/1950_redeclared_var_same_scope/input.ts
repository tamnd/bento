// JavaScript hoists a `var` to one binding per scope, so redeclaring a name with
// `var` in the same scope assigns the one variable again rather than introducing a
// new binding. Bento lowered each `var x = ...` to a Go short declaration, so the
// second `var x` tripped Go's "no new variables on left side of :=" and the build
// failed. A redeclaration now lowers to a plain assignment, so the later value wins.
var x = 1;
var x = 2;
console.log(x);
var s = "first";
var s = "second";
console.log(s);
