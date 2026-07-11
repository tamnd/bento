// The test262 async harness judges a test by the line its $DONE callback prints:
// "Test262:AsyncTestComplete" on success, a failure line otherwise. This fixture
// inlines that seam (the doneprintHandle.ts port the harness composes ahead of an
// async-flagged test) and drives an async test through it, so the async body's
// completion reaches the same console.log the judge reads. The failure branch
// probes 'name' in error, the in operator on a boxed caught value, which must
// compile for the whole $DONE function to lower even when the success path is taken.
function __consolePrintHandle__(msg: any): void {
  console.log(msg);
}

function $DONE(error?: any): void {
  if (error) {
    if (typeof error === "object" && error !== null && "name" in error) {
      __consolePrintHandle__("Test262:AsyncTestFailure:" + error.name + ": " + error.message);
    } else {
      __consolePrintHandle__("Test262:AsyncTestFailure:Test262Error: " + String(error));
    }
  } else {
    __consolePrintHandle__("Test262:AsyncTestComplete");
  }
}

async function step(): Promise<number> {
  return 1;
}

async function test(): Promise<number> {
  const a = await step();
  return a + 1;
}

test().then((v) => $DONE());
