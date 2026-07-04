// Set membership is SameValueZero, not the === a Go map key would use. Two NaNs
// count as the same member, so adding NaN twice leaves one, and +0 and -0 are the
// same member too, so adding both leaves one. With 1 added once as well, the three
// distinct members are 1, NaN, and zero, so size is three.
function run(): void {
  const s = new Set<number>();
  s.add(1);
  s.add(1);
  s.add(Number.NaN);
  s.add(Number.NaN);
  s.add(0);
  s.add(-0);
  console.log(s.size);
}

run();
