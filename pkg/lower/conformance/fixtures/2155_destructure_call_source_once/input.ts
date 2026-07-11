let calls = 0;
function make(): number[] {
  calls += 1;
  return [1, 2];
}
const [a, b] = make();
console.log(a + b);
console.log(calls);
