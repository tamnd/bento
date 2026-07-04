// Shorthand members {x} are the explicit {x: x}: the interned struct field takes
// the identifier read of the same name. Shorthand and explicit members share one
// shape, so a literal can mix them.
function run(): void {
  const first = "ada";
  const age = 36;
  const active = true;
  const person = { first, age, active };
  console.log(person.first);
  console.log(person.age);
  console.log(person.active);

  // shorthand next to an explicit member in the same literal
  const a = 10;
  const point = { a, b: 2 };
  console.log(point.a, point.b);
}

run();
