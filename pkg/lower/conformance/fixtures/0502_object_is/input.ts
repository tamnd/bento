// Object.is compares two values under SameValue, which parts from === at exactly
// two points: two NaNs are the same value and +0 and -0 are distinct. This
// lowers when the operands share one primitive type, so each pair reduces to the
// same-value test for that type. The NaN here comes from Math.sqrt(-1) and the
// negative zero from negating a runtime zero, since a bare NaN global does not
// lower and a -0 literal would fold to +0 as a Go constant.
function run(): void {
  const nan = Math.sqrt(-1);
  let z = 0;
  const nz = -z;

  console.log(Object.is(1, 1));
  console.log(Object.is(1, 2));

  // SameValue calls two NaNs equal where === does not
  console.log(Object.is(nan, nan));

  // and it keeps +0 and -0 apart where === merges them
  console.log(Object.is(z, nz));
  console.log(Object.is(nz, nz));

  console.log(Object.is("hi", "hi"));
  console.log(Object.is("hi", "ho"));

  console.log(Object.is(true, true));
  console.log(Object.is(true, false));
}

run();
