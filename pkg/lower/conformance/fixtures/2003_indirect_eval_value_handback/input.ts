// eval read as a value, not called, is an indirect eval. It is a function symbol
// in the lib, so the function-used-as-a-value path would capitalize it to an
// undeclared Eval and the generated Go would not build. bento has no eval, so the
// unit hands back to the interpreter instead of naming a symbol the runtime never
// declared, the same way a direct eval call already does.
var s = eval;
s("var x = 1");
