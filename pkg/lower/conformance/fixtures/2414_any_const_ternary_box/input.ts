// A ternary bound to an any const boxes into a value.Value rather than landing its
// branch primitive in the slot. The statement-position ternary would otherwise
// flatten into an if that assigns each branch raw, declaring the slot the branches'
// widened primitive, which a later dynamic read cannot take. The flatten bails on a
// dynamic binding, so the ordinary decl boxes the one ternary result the same way a
// plain any const boxes a literal.
function show(x: number): void {
  const s: any = x > 0 ? "u" : "d";
  const n: any = x > 0 ? 1 : 2;
  const b: any = x > 0 ? true : false;
  console.log(s);
  console.log(n);
  console.log(b);
}

show(1);
show(-1);
