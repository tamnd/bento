function pack(a: number, b: number, c: number): unknown {
  return arguments;
}
const args: any = pack(10, 20, 30);
console.log(args.length);
console.log(args[0], args[1], args[2]);
console.log(typeof args);
let total = 0;
for (let i = 0; i < args.length; i++) total += args[i];
console.log(total);

function countArgs(items: unknown): number {
  const a = items as any;
  return a.length;
}
function forward(x: number, y: number): number {
  return countArgs(arguments);
}
console.log(forward(7, 8));
