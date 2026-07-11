// Awaiting a rejected promise raises the rejection at the await, where a try/catch
// around the await catches it the same way a synchronous throw would. So the catch runs
// with the rejected error, the body recovers and returns a fallback, and the returned
// promise fulfills with that fallback rather than rejecting.
async function rejects(): Promise<number> {
  throw new Error("inner");
  return 0;
}

async function outer(): Promise<number> {
  try {
    return await rejects();
  } catch (e: any) {
    console.log("caught-in-body:" + e.message);
    return -1;
  }
}

outer().then((v) => console.log("result:" + v));
console.log("sync");
