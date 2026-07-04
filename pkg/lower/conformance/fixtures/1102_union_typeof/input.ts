// Narrowing a tagged-sum union with typeof is a single integer tag compare, not a
// runtime string build: typeof v === "string" lowers to v.tag == the string arm's
// constant, and inside the branch the checker has narrowed v to that member, so a
// read of v touches the arm field directly. A three-arm union number | string |
// boolean carries a tag, a field, and a constructor per arm, and each typeof test
// selects its own arm, so the whole switch is a chain of tag compares with no
// boxing and no reflection.
function describe(v: number | string | boolean): string {
  if (typeof v === "string") {
    return "string:" + v;
  }
  if (typeof v === "boolean") {
    if (v) {
      return "boolean:yes";
    }
    return "boolean:no";
  }
  return "number:" + String(v);
}

function run(): void {
  console.log(describe(42));
  console.log(describe("hi"));
  console.log(describe(true));
  console.log(describe(false));
  console.log(describe(0));
}

run();
