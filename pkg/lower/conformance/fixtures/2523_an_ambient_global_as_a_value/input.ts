const a = atob;
console.log("read", a === atob, typeof a, a.name);
console.log("global", globalThis.atob === atob);
console.log("call", a(btoa("bento")));

const run = (fn: any, arg: any) => fn(arg);
console.log("passed", run(btoa, "ok"));

const S = Symbol;
console.log("ctor", S === Symbol, typeof S, S("t") === S("t"));

const o = JSON.parse('{"a":1}');
console.log("coerce", Object(o) === o);

// Map is annotated any so the checker admits the call form the language throws on:
// the point of the case is what a program that calls a constructor without new sees
// at run time, and TypeScript rejects that spelling outright (2350).
const M: any = Map;
try {
  M();
} catch (e: any) {
  console.log("needs new", e.name, e.message);
}
