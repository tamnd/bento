// A module-level binding a top-level function reads cannot stay a local of main,
// since the function lowers to a package-level Go func that could not see a main
// local. Such a binding hoists to a package-level var so the function and the rest
// of main share one storage. A const is read across the boundary, a let is mutated
// across it and the mutation is read back, and a parameter that shadows a module
// name is resolved by its own symbol so it does not drag the module binding along.
const total = 100;
let count = 0;
const label = "hi";

function share(n: number): number {
  return total / n;
}

function bump(): void {
  count = count + 1;
}

function greet(name: string): string {
  return label + ", " + name;
}

function show(total: number): number {
  return total * 2;
}

console.log(share(4));
bump();
bump();
console.log(count);
console.log(greet("sam"));
console.log(show(10));
