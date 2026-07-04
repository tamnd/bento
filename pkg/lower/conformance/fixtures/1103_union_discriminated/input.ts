// A discriminated union of objects that share a string-literal tag property lowers
// to the section-9 tagged sum with pointer arms: each member interns to its own
// struct, the union carries an integer tag and one pointer field per arm, and a
// value is built through the arm constructor so the tag and the payload never drift.
// Narrowing is a single integer compare on the tag, in both the if form s.kind ===
// "circle" and the switch form switch (s.kind), and inside the branch a read of the
// value selects the arm's pointer field, so s.r becomes a direct field access with
// no runtime type test beyond the tag the branch already matched. The area switch
// has a case for every arm and no default, so the checker types its fall-out as
// never and the function ends in it with no trailing return; the lowering carries
// that across with a synthesized default that panics, unreachable in well-typed code.
interface Circle {
  kind: "circle";
  r: number;
}

interface Square {
  kind: "square";
  side: number;
}

interface Rect {
  kind: "rect";
  w: number;
  h: number;
}

type Shape = Circle | Square | Rect;

function area(s: Shape): number {
  switch (s.kind) {
    case "circle":
      return 3 * s.r * s.r;
    case "square":
      return s.side * s.side;
    case "rect":
      return s.w * s.h;
  }
}

function label(s: Shape): string {
  if (s.kind === "circle") {
    return "circle";
  }
  if (s.kind === "square") {
    return "square";
  }
  return "rect";
}

function run(): void {
  const c: Shape = { kind: "circle", r: 2 };
  const sq: Shape = { kind: "square", side: 3 };
  const rc: Shape = { kind: "rect", w: 4, h: 5 };
  console.log(label(c) + ":" + String(area(c)));
  console.log(label(sq) + ":" + String(area(sq)));
  console.log(label(rc) + ":" + String(area(rc)));
}

run();
