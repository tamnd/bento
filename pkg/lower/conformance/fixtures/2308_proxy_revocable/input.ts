// Proxy.revocable pairs a proxy with a revoke function as a { proxy, revoke } object.
// The proxy forwards to the target until revoke is called, after which every
// operation on it throws a TypeError.
const target: any = { x: 1 };
const r: any = Proxy.revocable(target, {});
const p: any = r.proxy;
const revoke: any = r.revoke;
console.log(p.x); // 1
revoke();
try {
  console.log(p.x);
} catch (e: any) {
  console.log("threw"); // threw
}
