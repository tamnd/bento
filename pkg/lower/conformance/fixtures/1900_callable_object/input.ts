// The test262 assert prelude binds a value that is both callable and carries
// named helpers: `const assert = function () {} as Assert`, then hangs each
// check off it. This exercises that shape on its own. The interface has a call
// signature, a data property, and a method, all reached the way the prelude
// reaches them: call the value, read a field, assign a method after the bind,
// and call that method.
interface Counter {
    (label: string): void;
    total: number;
    bump(by: number): void;
}

const counter = function (label: string): void {
    console.log(label);
} as Counter;

counter.total = 5;
counter.bump = function (by: number): void {
    console.log(by);
};

counter("start");
counter.bump(3);
console.log(counter.total);
