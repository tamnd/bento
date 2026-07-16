// A binding declared with an optional type and no initializer holds undefined until
// its first assignment, the way a JavaScript optional reads before it is given a
// defined value. It lowers to var x value.Opt[T], whose Go zero value is the
// undefined case, so the fresh binding reads undefined on its own, accepts a later
// assignment, and flows where the same T | undefined is expected with no narrowing.
function pick(v: string | undefined): string {
  return v ?? "fallback";
}

function run(): void {
  // A fresh optional string reads undefined, so pick returns its fallback.
  let s: string | undefined;
  console.log(pick(s));

  // After an assignment the same binding carries its value where the optional is
  // expected.
  s = "hi";
  console.log(pick(s));

  // A fresh optional number reads undefined on a presence test.
  let n: number | undefined;
  if (n === undefined) {
    console.log("n-undefined");
  }
}

run();
