export function lengths(n: number): number {
  const xs = [n, n + 1, n + 2];
  const dashed = xs.join("-");
  const defaulted = xs.join();
  const words = ["a", "bb", "ccc"];
  const joinedWords = words.join(", ");
  return dashed.length + defaulted.length + joinedWords.length;
}
