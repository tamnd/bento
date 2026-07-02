export function find(n: number): number {
  const xs = [10, 20, 30, 40];
  const at = xs.indexOf(n);
  let score = at;
  if (xs.includes(n)) {
    score = score + 1000;
  }
  const words = ["a", "bb", "ccc"];
  score = score + words.indexOf("bb");
  return score;
}
