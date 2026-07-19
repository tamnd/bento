// Audit wave W21a: a conditional whose branches are unlike literal types, a string
// literal and a number literal, is typed by the checker as their literal union
// ("hi" | 7), which does not intern as a base-primitive tagged sum on its own. The
// literal members widen to their base arms, so the union interns the same NumOrStr a
// string | number does, and each branch boxes into its arm. The value is then read
// through the truthiness and string forms every arm grows, so it is observable end to
// end with no runtime trace of the literal-ness.

function pick(useStr: boolean) {
  const x = useStr ? "hi" : 7;
  return x;
}

// The true branch is the string literal arm.
console.log(pick(true));
// The false branch is the number literal arm.
console.log(pick(false));

// A mixed literal union read for truthiness: the empty-string and zero literals are the
// falsy members of each arm, so the tagged sum's ToBoolean must switch on the tag.
function truthy(pickStr: boolean, s: "" | "x", n: 0 | 1): string {
  const v = pickStr ? s : n;
  if (v) return "yes";
  return "no";
}

console.log(truthy(true, "x", 0));
console.log(truthy(true, "", 0));
console.log(truthy(false, "x", 1));
console.log(truthy(false, "x", 0));
