// a[i] = v writes an element in place. A write inside the array overwrites the
// element and leaves the length alone, and a write at the current length extends
// the array by one, the a[a.length] = v append idiom. The value reaches the
// element type, so a string array takes a string write. This fixture stays inside
// the array and appends at the end, the writes that match JavaScript exactly, and
// leaves the grow-past-the-end holes for their own slice.
function run(): void {
  const a: number[] = [1, 2, 3];
  a[0] = 10;
  a[2] = 30;
  a[a.length] = 4;
  for (let i = 0; i < a.length; i++) {
    console.log(a[i]);
  }
  console.log(a.length);

  const s: string[] = ["a", "b"];
  s[1] = "B";
  s[s.length] = "c";
  console.log(s.join(","));
}

run();
