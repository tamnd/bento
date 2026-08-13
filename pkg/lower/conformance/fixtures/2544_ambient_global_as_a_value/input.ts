// A global named rather than called is a value: it reports itself as a function,
// carries its own name, and is the same object wherever the program reaches it.
const knownGlobals: unknown[] = [
  AbortController,
  atob,
  btoa,
  clearImmediate,
  clearInterval,
  clearTimeout,
  global,
  setImmediate,
  setInterval,
  setTimeout,
  queueMicrotask,
  structuredClone,
];
console.log(new Set(knownGlobals).size, knownGlobals.length);

// The same name read twice is the same object, and so is the one read off the
// global scope, which is what a known-globals set is for.
console.log(atob === globalThis.atob, AbortController === globalThis.AbortController);

// global is Node's own second name for the global scope, not a second object.
console.log(global === globalThis, typeof global);

// The Node classes report themselves the way any function does.
console.log(typeof AbortController, typeof AbortSignal, typeof Event, typeof EventTarget);

// A global handed to a helper is still the global inside it.
function isSame(a: unknown, b: unknown): boolean {
  return a === b;
}
console.log(isSame(setTimeout, globalThis.setTimeout), isSame(atob, btoa));

// Constructing one directly is unchanged by any of that.
const controller = new AbortController();
console.log(controller.signal.aborted);
controller.abort();
console.log(controller.signal.aborted, controller.signal.reason.name);
