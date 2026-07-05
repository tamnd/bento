// A chained assignment a = b = 5 evaluates the right side once and settles every
// target with that value, so it lowers to the innermost assignment followed by an
// outward copy per target, b = 5; a = b. The right side runs once even with two
// targets, which the single logged "call" line shows: were it run per target the
// line would repeat.
function next(): number {
  console.log("call");
  return 7;
}

function run(): void {
  let a = 0;
  let b = 0;
  let c = 0;
  a = b = c = 5;
  console.log(a + "," + b + "," + c);

  let x = 0;
  let y = 0;
  x = y = next();
  console.log(x + "," + y);
}

run();
