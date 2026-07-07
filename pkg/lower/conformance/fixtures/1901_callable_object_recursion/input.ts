// The assert prelude's call body reads the object back (it reaches for its own
// message helpers), so the closure has to see the same object the binding names.
// Go gets that from declaring the pointer before the closure captures it, which
// is what the two-step lowering sets up. Here the call body reads a field the
// binding fills in after the bind, and it still sees the filled-in value.
interface Logger {
    (msg: string): void;
    prefix: string;
}

const log = function (msg: string): void {
    console.log(log.prefix + msg);
} as Logger;

log.prefix = ">> ";
log("hello");
log("world");
