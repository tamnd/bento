// A function's .name is its bound name, the source identifier a declaration binds.
// bento models a function as a bare Go func with no struct, so a read of .name would
// fold to undefined the way a missing struct field does. It lowers instead to the
// string constant of the declared name, what JavaScript reports for a named function.
function add(a: number, b: number): number {
  return a + b;
}

function greet(name: string): string {
  return "hi " + name;
}

console.log(add.name);
console.log(greet.name);
