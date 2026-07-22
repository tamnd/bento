async function run(): Promise<void> {
  const pairs: Promise<[number, string]>[] = [
    Promise.resolve([1, "a"] as [number, string]),
    Promise.resolve([2, "b"] as [number, string]),
  ];
  for await (const [n, s] of pairs) {
    console.log(n + ":" + s);
  }
  console.log("done");
}
run();
