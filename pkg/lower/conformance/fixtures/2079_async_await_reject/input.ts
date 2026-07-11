// A throw inside an async body that has already awaited rejects the promise the body
// returned: the coroutine unwinds the throw and settles its promise as rejected, so a
// .catch on the returned promise observes the thrown error. Like every settled reaction
// it runs at the microtask checkpoint, after the trailing synchronous line.
async function base(): Promise<number> {
  return 1;
}

async function boom(): Promise<number> {
  const a = await base();
  if (a > 0) {
    throw new Error("bang:" + a);
  }
  return a;
}

boom().catch((e) => console.log("caught:" + e.message));
console.log("sync");
