// exec and test coerce their subject the way ToString does, so a subject the checker
// does not call a string still matches against the text it renders to.
const text: any = "one two three";
console.log(/two/.test(text));
console.log(/four/.test(text));
console.log(/(t\w+)/.exec(text)?.[1]);
console.log(/(t\w+)/.exec(text)?.index);
console.log(/nope/.exec(text)?.[0]);

const n: any = 1234;
console.log(/23/.test(n));
console.log(/^\d+$/.test(n));
console.log(/(\d\d)$/.exec(n)?.[1]);

const b: any = true;
console.log(/rue/.test(b));

const nul: any = null;
console.log(/null/.test(nul));

const und: any = undefined;
console.log(/undefined/.test(und));

const arr: any = [1, 2, 3];
console.log(/1,2/.test(arr));

const obj: any = { toString(): string { return "made up"; } };
console.log(/made/.test(obj));
console.log(/(\w+) up/.exec(obj)?.[1]);

const parsed = JSON.parse("\"ab12\"");
console.log(/\d+/.test(parsed));
console.log(/([a-z]+)/.exec(parsed)?.[1]);

const mixed: any[] = [77, "seven", false];
console.log(/7/.test(mixed[0]));
console.log(/even/.test(mixed[1]));
console.log(/als/.test(mixed[2]));

const built = new RegExp("t(w)o");
console.log(built.test(text));
console.log(built.exec(text)?.[1]);

const empty: any = "";
console.log(/^$/.test(empty));
