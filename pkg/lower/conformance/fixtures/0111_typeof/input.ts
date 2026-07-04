// typeof reports a value's JavaScript type as one of a fixed set of strings. When
// the checker already knows the operand's kind the result folds to that string at
// compile time and the operand is not evaluated again; a dynamic operand carries
// its kind on the boxed value and reports it at runtime. The two results that
// surprise people are null, which answers "object", and an array, which answers
// "object" like any other object rather than a kind of its own.
function staticNumber(n: number): string {
  return typeof n;
}

function staticString(s: string): string {
  return typeof s;
}

function staticBool(b: boolean): string {
  return typeof b;
}

function staticBig(b: bigint): string {
  return typeof b;
}

function staticFn(f: () => number): string {
  return typeof f;
}

function kindOf(x: any): string {
  return typeof x;
}

function run(): void {
  console.log(staticNumber(1));
  console.log(staticString("a"));
  console.log(staticBool(true));
  console.log(staticBig(2n));
  console.log(staticFn(() => 1));
  console.log(kindOf(JSON.parse("1")));
  console.log(kindOf(JSON.parse("\"a\"")));
  console.log(kindOf(JSON.parse("true")));
  console.log(kindOf(JSON.parse("null")));
  console.log(kindOf(JSON.parse("[1, 2]")));
  console.log(kindOf(JSON.parse("{}")));
}

run();
