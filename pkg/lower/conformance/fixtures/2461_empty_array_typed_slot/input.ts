// A bare [] carries no elements, so the checker types it never[], with an element
// type of never rather than the type the slot it flows into declares. On its own that
// lowers to value.NewArray[value.Value](), a *value.Array[value.Value] that a typed
// array slot rejects at go build. When the empty literal flows into a slot whose type
// the checker already knows, an argument, a return, or an assignment, it takes the
// slot's element type instead, the same contextual typing an object literal argument
// gets, so NewArray is spelled at the slot's element type and the value crosses cleanly.

// The empty literal as a call argument adopts the parameter's element type.
function count(a: number[]): number {
  return a.length;
}
console.log(count([1, 2, 3]));
console.log(count([]));

// The empty literal as a return value adopts the function's return element type.
function empty(): string[] {
  return [];
}
console.log(empty().length);

// The empty literal reassigned into a typed binding adopts its element type.
let xs: number[] = [10, 20];
xs = [];
console.log(xs.length);

// A non-number element type carries through the same way.
function names(a: string[]): number {
  return a.length;
}
console.log(names([]));
