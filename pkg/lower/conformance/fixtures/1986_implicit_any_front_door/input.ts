// The AOT front door tolerates strict mode's implicit-any reports rather than
// refusing the form: an untyped function reads its parameters through the
// dynamic value path, inferred-any locals carry a number and a string forward,
// and an untyped class member holds state behind an untyped getter. None of
// these carry a type annotation, so strict mode flags each as implicitly `any`,
// but the resolved type is already `any` and every one lowers to a dynamic slot.

function add(a, b) {
  return a + b;
}

let sum;
sum = add(2, 3);

let joined;
joined = add("a", "b");

class Box {
  contents = 10;
  get twice() { return this.contents + this.contents; }
}

console.log(sum);
console.log(joined);
console.log(new Box().twice);
