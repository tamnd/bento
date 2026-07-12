// The multi-agent concurrency ceiling. Atomics.wait blocks one agent until another
// agent calls Atomics.notify on the same shared memory, so a test that drives a wait
// through to an "ok" result, or a notify that wakes a real waiter, spawns that second
// agent through a host hook that runs a source string as a new agent. bento's AOT output
// is a single Go process with one agent, so it provides no host hook to start a second
// one: a program that calls that hook has no body to lower and hands back rather than
// pretend a second agent ran and sent a notify. The single-agent Atomics.wait and notify
// do lower and run, wait reporting not-equal or timed-out and notify waking zero; only a
// test that needs a real second agent to make progress does not.
declare function startAgent(source: string): void;

function run(): void {
  const i32a = new Int32Array(new SharedArrayBuffer(4));

  startAgent(`
    const shared = new Int32Array(receiveSharedBuffer());
    Atomics.store(shared, 0, 42);
    Atomics.notify(shared, 0, 1);
  `);

  // With a real second agent this wait would block and then resolve to "ok" once the
  // notify above arrives. Single agent there is no such agent, so the program cannot be
  // honored and hands back at the startAgent call above.
  const result: string = Atomics.wait(i32a, 0, 0);
  console.log(result);
}

run();
