// A value-returning || or && on a dynamic operand whose right side has a side
// effect must evaluate the right only when the operator does not short-circuit:
// || reaches the right when the left is falsy, && when the left is truthy. The
// eager value.Or and value.And helpers take both operands already evaluated, so a
// right side with an effect could not short-circuit and handed back. The lazy form
// evaluates the left once, tests its truthiness, and touches the right only when it
// is reached, so the effect fires exactly when JavaScript says it does.

let calls = 0;
function effect(v: any): any {
  calls++;
  return v;
}

const a: any = "keep";
const r1 = a || effect("fallback");
console.log(r1, calls);

const b: any = "";
const r2 = b || effect("fallback");
console.log(r2, calls);

const c: any = 0;
const r3 = c && effect("reached");
console.log(r3, calls);

const d: any = "go";
const r4 = d && effect("reached");
console.log(r4, calls);
