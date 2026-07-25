// A function whose only arguments read is arguments.length may write a named
// parameter without handing back: the length is fixed by the call arity and does not
// track the parameter in either the mapped or the unmapped arguments object, so the
// entry snapshot stays faithful. It is the shape node:assert's fail() reaches, a
// length test guarding a rewrite of its own parameters. An element read (arguments[i])
// alongside a parameter write would still hand back, since that value does alias.
function label(name: any): string {
  if (arguments.length === 1) {
    name = "[" + name + "]";
  }
  return String(name);
}
function pair(a: any, b: any): string {
  if (arguments.length === 1) {
    b = a;
  }
  return String(a) + "," + String(b);
}
console.log(label("x"));
console.log(pair("p", "q"));
