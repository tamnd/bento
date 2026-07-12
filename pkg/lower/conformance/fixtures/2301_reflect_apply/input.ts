// Reflect.apply spreads an array-like argument list into a call of the target and
// returns the result. bento functions do not read this, so the thisArgument slot is
// filled but consulted no further than the argument order requires.
const sum: any = (a: number, b: number, c: number): number => a + b + c;
console.log(Reflect.apply(sum, undefined, [1, 2, 3])); // 6
console.log(Reflect.apply(sum, null, [10, 20, 30])); // 60

// The argument list is read positionally, so a longer list passes every element.
const cat: any = (a: string, b: string, c: string): string => a + "-" + b + "-" + c;
console.log(Reflect.apply(cat, undefined, ["x", "y", "z"])); // x-y-z
