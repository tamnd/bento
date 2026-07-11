// A promise that rejects and is never observed is an unhandled rejection. Once the
// synchronous run finishes and the microtask queue drains, the runtime reports it and
// exits non-zero, the way JavaScript surfaces an unhandledrejection after the checkpoint.
// A test that asserts a rejection observes it through this crash rather than a false pass.
// The synchronous log still reaches stdout, since the report runs only after the run ends.
const failing: Promise<number> = Promise.reject("boom");
console.log("sync");
