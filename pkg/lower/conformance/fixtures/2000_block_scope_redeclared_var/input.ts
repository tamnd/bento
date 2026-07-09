// JavaScript scopes a var to the whole function and lets it redeclare a name in the
// same scope, so `{ var f; var f; }` is one binding with two declaration name nodes.
// The second var lowers to nothing, so the first must carry the trailing blank an
// unread binding needs, else the emitted Go trips declared-and-not-used. Counting the
// two name nodes as two uses would leave the binding unblanked; the lowerer takes the
// binding's declaration count as the baseline so a redeclared-but-unread var is still
// recognized as unused and blanked exactly once. test262 reaches this with the
// block-scope redeclaration-attempt-with-var positive tests.
{ var f; var f; }
console.log("ok");
