// A bare block introduces its own lexical scope, so a binding it declares is visible
// only inside the braces and a same-named binding in a later block is a separate
// variable. Go spells the lexical scope the same way, so a standalone block lowers
// to a Go block.
function scopedSum(): number {
  let s = 0;
  {
    let x = 10;
    s = s + x;
  }
  {
    let x = 20;
    s = s + x;
  }
  return s;
}

function run(): void {
  console.log(scopedSum());
}

run();
