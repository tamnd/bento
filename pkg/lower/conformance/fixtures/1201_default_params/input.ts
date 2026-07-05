// A default parameter lets a caller omit the argument. Go has no optional
// arguments, so the parameter lowers to a plain Go field of its type and every call
// fills the omitted slot with the default, while a provided argument passes through.
// A numeric, string, and boolean default all ride across the boundary, and a
// function with two defaults fills only the slots a call leaves off.
function inc(x: number, by: number = 1): number {
  return x + by;
}

function greet(name: string, greeting: string = "hi"): string {
  return greeting + ", " + name;
}

function box(w: number = 2, h: number = 3): number {
  return w * h;
}

function pick(x: number, on: boolean = true): number {
  return on ? x : 0;
}

console.log(inc(5));
console.log(inc(5, 3));
console.log(greet("sam"));
console.log(greet("sam", "yo"));
console.log(box());
console.log(box(5));
console.log(box(5, 6));
console.log(pick(9));
console.log(pick(9, false));
