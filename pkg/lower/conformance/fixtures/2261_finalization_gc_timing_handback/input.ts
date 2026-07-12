// The garbage-collection observability ceiling. A FinalizationRegistry's cleanup
// callback runs after its target is collected, but a test that pins exactly when that
// happens drives a host garbage collector and then asserts the callback has run on the
// same turn. The AOT model runs cleanups on Go's own cleanup goroutine after a
// collection, whose turn it cannot force, and it provides no host gc hook, so a program
// that calls one has no body to lower and hands back rather than pretend a collection
// happened. The register and unregister surface lowers; only this timing does not.
declare function gc(): void;

function run(): void {
  const registry = new FinalizationRegistry<string>((held: string) => {
    console.log("collected " + held);
  });

  let target: { id: number } | null = { id: 1 };
  registry.register(target, "one");
  target = null;

  gc();
  console.log("after gc");
}

run();
