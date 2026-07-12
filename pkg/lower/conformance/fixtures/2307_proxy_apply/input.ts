// An apply trap intercepts a call to a callable proxy, receiving the target, the
// this value, and the arguments as one array, so the proxy can rewrite the call
// before it reaches the wrapped function.
const target: any = (a: number, b: number): number => a + b;
const p: any = new Proxy(target, {
  apply: (t: any, thisArg: any, argsList: any): any => {
    const r: any = t(argsList[0], argsList[1]) * 10;
    return r;
  },
});
console.log(p(2, 3)); // 50

// A callable proxy with no apply trap forwards the call to the target unchanged.
const q: any = new Proxy(target, {});
console.log(q(4, 5)); // 9
