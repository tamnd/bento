// A method call on the result of Map.get reads a value the checker types as
// `V | undefined`, because it cannot prove the key is present, and reports 18048
// ("'X' is possibly 'undefined'"). That report is a strictness artifact over
// JavaScript, which returns the stored value at run time when the key is present.
// The front door tolerates the report so the program reaches the renderer, but the
// renderer lowers a method call only over a receiver whose static type carries it,
// and the un-narrowed `V | undefined` receiver carries none, so it hands back to the
// engine with its own named reason rather than emitting a wrong call.
const m = new Map<number, number>([[1, 2]]);
const v = m.get(1);
console.log(v.toString());
