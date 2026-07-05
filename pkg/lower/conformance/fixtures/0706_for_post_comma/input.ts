// A for loop's post clause holds exactly one Go statement, so a comma of updates
// cannot lower to two of them. Its operands fuse into one parallel assignment,
// i, j = i + 1, j - 1, which advances every target together the way the comma
// sequence does. A two-pointer walk that steps both ends toward the middle is the
// idiom this covers, with a plain increment and with a compound step.
function meetInMiddle(): number {
  let steps = 0;
  for (let i = 0, j = 7; i < j; i++, j--) {
    steps = steps + 1;
  }
  return steps;
}

function stridedPair(): number {
  let hits = 0;
  for (let i = 0, j = 10; i < j; i += 2, j -= 2) {
    hits = hits + 1;
  }
  return hits;
}

function threeCounters(): number {
  let last = 0;
  for (let i = 0, j = 9, k = 0; i < j; i++, j--, k++) {
    last = k;
  }
  return last;
}

function run(): void {
  console.log(meetInMiddle());
  console.log(stridedPair());
  console.log(threeCounters());
}

run();
