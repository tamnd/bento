let a: number | null = null;
a ??= 5;
console.log(a);

let b: number | null = 7;
b ??= 99;
console.log(b);

let c: string | null | undefined = null;
c ??= "x";
console.log(c);

let d: string | null | undefined = undefined;
d ??= "y";
console.log(d);

let e: string | null | undefined = "keep";
e ??= "z";
console.log(e);

let hits = 0;
function bump(): number {
  hits++;
  return 1;
}
let f: number | null = 3;
f ??= bump();
console.log(f, hits);
