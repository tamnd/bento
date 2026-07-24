// A ternary whose branches are an any-typed value and a concrete primitive types
// the whole expression any, so no tagged sum spells its result. Each branch bridges
// to a value.Value the way an argument crossing into an any parameter does: the
// any branch passes through and the primitive branch boxes. It is the shape the
// path module's resolve() reaches (`i >= 0 ? arguments[i] : fallback`), here written
// with an any parameter so the oracle stays to plain value rendering.
function pick(cond: boolean, a: any): void {
  const s: any = cond ? a : "fallback";
  const n: any = cond ? 7 : a;
  console.log(s);
  console.log(n);
}

pick(true, "given");
pick(false, 42);
