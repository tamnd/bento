// Inside a specialization the bare type parameter resolves to the concrete type
// the call fixed, not only in the parameter and return but everywhere the body
// spells it: a local annotated T reads as the concrete Go type, and a T[] built
// in the body becomes value.Array of that type. box(5) specializes to Box_num,
// whose first local is a float64 and whose pair is *value.Array[float64];
// box("hi") specializes to Box_str over value.BStr. The specialization carries
// one substitution the whole body lowers under, so every T resolves the same way.
function box<T>(x: T): T[] {
  const first: T = x;
  const pair: T[] = [first, x];
  return pair;
}

console.log(box(5).length);
console.log(box("hi").length);
