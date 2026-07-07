// A local declared and never read still runs its initializer in JavaScript, so
// the side effect of `var x = tick();` happens even though nothing reads x. Go
// rejects a local that is declared and not used, so the lowering keeps the
// binding and follows it with a blank assignment. Many test262 tests take this
// shape, a lone `var x = <expr>;` whose value is never inspected, so clearing
// this wall is what lets the prelude-backed tests reach their own bodies.
let count: number = 0;
function tick(): number {
  count += 1;
  return count;
}
var first = tick();
var second = tick();
let third = "unused";
console.log(String(count));
