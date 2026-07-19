// The empty statement, a lone semicolon, is grammatical wherever a statement is
// expected and does nothing. The debugger statement pauses execution when a debugger
// is attached and is a no-op otherwise; an AOT binary has none, so it does nothing
// too. Both lower to no Go statement at all, so the surrounding real statements run
// unchanged and the emitted body reads as if the no-ops were never written.

function run(a: number): number {
  ;
  let total = a;
  debugger;
  total = total + 1;
  ;
  return total;
}

console.log(run(41));

// A lone semicolon between top-level statements is dropped the same way.
let count = 0;
;
count = count + 5;
debugger;
console.log(count);
