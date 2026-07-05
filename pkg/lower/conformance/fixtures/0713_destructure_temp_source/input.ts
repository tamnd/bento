// Destructuring off a source that is not a plain variable holds the source in a
// generated temporary read once, then reads each name off that temporary. A plain
// variable repeats cost-free, but a call would run once per name if repeated, so the
// call is evaluated a single time into the temporary and every element or property
// selects off the held value. This holds for both array and object patterns.
function pair(): number[] {
  return [10, 20];
}
const [a, b] = pair();
console.log(a + b);

function point(): { x: number; y: number } {
  return { x: 3, y: 4 };
}
const { x, y } = point();
console.log(x * y);

// A method call source is held once too, so a split runs a single time.
const csv = "one,two,three";
const [first, second] = csv.split(",");
console.log(first + "-" + second);
