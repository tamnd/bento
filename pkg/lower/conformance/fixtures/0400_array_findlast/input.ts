// findLast and findLastIndex are the from-the-end siblings of find and findIndex:
// they visit the array in descending index order and return the first match they
// meet, which is the last match in forward order. findLast returns the element or
// undefined, findLastIndex returns the index or -1.
function lastEven(a: number[]): number | undefined {
  return a.findLast((n) => n % 2 === 0);
}

function lastEvenIndex(a: number[]): number {
  return a.findLastIndex((n) => n % 2 === 0);
}

function run(): void {
  const a = [1, 2, 3, 4, 5, 6];
  const found = lastEven(a);
  if (found !== undefined) {
    console.log(found);
  }
  console.log(lastEvenIndex(a));

  const odds = [1, 3, 5];
  console.log(lastEven(odds) === undefined);
  console.log(lastEvenIndex(odds));
}

run();
