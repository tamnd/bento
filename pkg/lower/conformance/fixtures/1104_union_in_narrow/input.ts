// The in operator narrows a discriminated object union to the arms that carry the
// named property, and lowers to a tag test rather than a runtime property probe.
// "radius" in s over a circle | square union, where only the circle arm has a
// radius, lowers to s.tag == the circle constant, and inside the branch the checker
// has narrowed s to that arm, so s.radius reads the arm pointer field directly.
// A union member unique to one arm makes in a one-integer compare, the same cost as
// the discriminant test, with no map lookup and no boxing.
interface Circle {
  kind: "circle";
  radius: number;
}

interface Square {
  kind: "square";
  side: number;
}

type Shape = Circle | Square;

function area(s: Shape): number {
  if ("radius" in s) {
    return 3 * s.radius * s.radius;
  }
  return s.side * s.side;
}

function run(): void {
  const c: Shape = { kind: "circle", radius: 2 };
  const sq: Shape = { kind: "square", side: 3 };
  console.log(String(area(c)));
  console.log(String(area(sq)));
}

run();
