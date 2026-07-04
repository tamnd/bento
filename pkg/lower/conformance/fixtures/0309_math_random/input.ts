// Math.random returns a fresh float in [0, 1) on every call, so the TypeScript and
// the lowered Go draw unrelated numbers and their raw output never matches. This
// fixture proves its shape instead: it prints only facts both runtimes satisfy,
// that every draw over a large sample lands in [0, 1) and that the draws are not
// all the same value. Both sides print the same two booleans, so the differential
// oracle holds without ever comparing a raw draw.
function run(): void {
  let allInRange = true;
  let allSame = true;
  const first = Math.random();
  for (let i = 0; i < 1000; i++) {
    const r = Math.random();
    if (r < 0 || r >= 1) {
      allInRange = false;
    }
    if (r !== first) {
      allSame = false;
    }
  }
  console.log(allInRange);
  console.log(allSame);
}

run();
