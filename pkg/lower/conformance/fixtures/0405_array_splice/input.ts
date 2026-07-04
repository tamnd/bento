// splice removes a run of elements and inserts new ones in their place, in one
// pass, returning the removed elements as a new array. The start is a relative
// index, so a negative one counts from the end, and the delete count clamps so it
// never runs past the end. With the count omitted it removes everything from the
// start onward. The receiver is changed in place.
function run(): void {
  const a = [1, 2, 3, 4, 5];
  console.log(a.splice(1, 2).join(","));
  console.log(a.join(","));

  const b = [1, 2, 3];
  console.log(b.splice(1, 1, 9, 8, 7).join(","));
  console.log(b.join(","));

  const c = [1, 2, 3];
  console.log(c.splice(1, 0, 42).length);
  console.log(c.join(","));

  const d = [1, 2, 3, 4, 5];
  console.log(d.splice(-2, 1).join(","));
  console.log(d.join(","));

  const e = [1, 2, 3, 4, 5];
  console.log(e.splice(2).join(","));
  console.log(e.join(","));
}

run();
