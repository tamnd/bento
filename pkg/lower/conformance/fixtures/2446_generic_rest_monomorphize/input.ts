// A generic function can fix its type parameter through the rest parameter alone.
// firstOf<T>(...xs: T[]): T binds T from the trailing array and returns a bare T, so
// bento monomorphizes one Go function per concrete element type: the rest lowers to a
// *value.Array[T] field and the specialization body reads xs[0] as the concrete type.
// A print of a bare T inside the body resolves the type parameter to the concrete
// type, so the number instantiation stringifies through NumberToString and the string
// one prints its value.BStr directly. A call whose string-literal arguments the
// checker infers as a union ("lo" | "hi") folds to value.BStr the same way a
// numeric-literal union folds to float64, so it shares the str specialization.
function firstOf<T>(...xs: T[]): T {
  return xs[0];
}
function logAll<T>(...xs: T[]): void {
  for (const x of xs) {
    console.log(x);
  }
}
console.log(firstOf(10, 20, 30)); // 10, the number instantiation
console.log(firstOf("lo", "hi")); // lo, a string-literal-union instantiation
logAll(1, 2, 3); // 1 2 3, T stringified through NumberToString
logAll("a", "b"); // a b, T printed as value.BStr directly
