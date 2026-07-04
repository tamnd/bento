// A labeled statement names a loop so a break or continue inside a nested loop can
// target the outer one rather than the innermost. It lowers to the Go labeled
// statement, and a labeled break or continue lowers to the Go branch that names the
// same label. A labeled continue skips to the next iteration of the outer loop; a
// labeled break leaves the whole nest. A label the body never targets is dropped,
// since Go rejects an unused label while JavaScript accepts it.
function skipInnerRest(): number {
  let n = 0;
  outer: for (let i = 0; i < 3; i = i + 1) {
    for (let j = 0; j < 3; j = j + 1) {
      if (j === 1) {
        continue outer;
      }
      n = n + 1;
    }
  }
  return n;
}

function leaveNest(): number {
  let n = 0;
  top: for (let i = 0; i < 3; i = i + 1) {
    for (let j = 0; j < 3; j = j + 1) {
      if (i === 1 && j === 1) {
        break top;
      }
      n = n + 1;
    }
  }
  return n;
}

function unusedLabel(): number {
  let n = 0;
  loop: for (let i = 0; i < 3; i = i + 1) {
    n = n + 1;
  }
  return n;
}

function run(): void {
  console.log(skipInnerRest());
  console.log(leaveNest());
  console.log(unusedLabel());
}

run();
