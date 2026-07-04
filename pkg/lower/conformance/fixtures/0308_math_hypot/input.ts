// Math.hypot folds any number of arguments through value.HypotN, the pairwise
// sqrt-of-sum-of-squares fold that Go's two-argument math.Hypot cannot express on
// its own. The three-argument call is the form that proves the variadic path:
// hypot(3, 4, 12) is 13 because 9 + 16 + 144 is 169.
function dist(x: number, y: number, z: number): number {
  return Math.hypot(x, y, z);
}

console.log(dist(3, 4, 12));
