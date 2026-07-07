// The four relational operators over dynamic operands run the Abstract Relational
// Comparison: two strings order by code unit, everything else compares as numbers,
// and a NaN operand makes all four false.
function lt(a: any, b: any): boolean { return a < b; }
function le(a: any, b: any): boolean { return a <= b; }
function gt(a: any, b: any): boolean { return a > b; }
function ge(a: any, b: any): boolean { return a >= b; }

console.log(lt(1, 2));
console.log(lt(2, 2));
console.log(le(2, 2));
console.log(gt(3, 2));
console.log(ge(2, 2));
console.log(lt("a", "b"));
console.log(lt("10", "9"));
console.log(lt(10, 9));
console.log(lt(1, "2"));
console.log(le(NaN, 1));
console.log(ge(NaN, 1));
console.log(gt("b", "a"));
console.log(ge("b", "b"));
