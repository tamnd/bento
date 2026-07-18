const b: bigint = 5n;
const d: any = b;
console.log(d);
const wide: any = 18446744073709551616n;
console.log(wide);
function add(): any {
  return 7n + 3n;
}
console.log(add());
console.log(String(d));
console.log(`${d}`);
