// A local read only through a `typeof` that the lowerer folds to a static tag has no
// surviving read in the emitted Go: the fold replaces the read with a constant string,
// so the binding looks declared-and-not-used. The lowerer counts a folded typeof as an
// elided read and marks the binding used with the trailing blank, so the initializer
// (here a function expression) still runs and the compare still holds. test262 reaches
// this with `if (typeof result !== "function")` over a function-valued local.
var result = function f(o: any) {
  o.x = 1;
  return o;
};
if (typeof result !== "function") {
  throw new Error("not a function");
}
console.log("ok");
