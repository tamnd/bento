package build

import "testing"

// TestPromiseAllOverATupleReadsLikeNode is the capability this is for. `await
// Promise.all([a(), b()])` is the everyday way to run two async calls at once, and it did
// not build: with nothing to widen the literal, the checker keeps one element type per
// position, so the call produces a Promise of a tuple rather than of an array, and
// value.All takes one element type for the whole slice.
//
// The lines worth reading twice are the first and the last. The first mixes a
// Promise<number> with a Promise<string> in one call, which is the shape a Go slice cannot
// hold and the reason the tuple path exists at all. The last prints a tuple, which used to
// read {"E0":1,"E1":"a"} because a tuple's Go shape is a positional struct and the boxing
// path claimed it as an object.
//
// This builds a real binary and holds its whole output against what Node v24.18.0 prints.
func TestPromiseAllOverATupleReadsLikeNode(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"async function num(x: number): Promise<number> { return x * 2; }\n"+
			"async function str(s: string): Promise<string> { return s + '!'; }\n"+
			"async function obj(n: number): Promise<{ n: number }> { return { n }; }\n"+
			"async function main() {\n"+
			"  const [a, b] = await Promise.all([num(1), str('x')]);\n"+
			"  console.log(a, b);\n"+
			"  const [c, d, e] = await Promise.all([num(2), num(3), num(4)]);\n"+
			"  console.log(c, d, e);\n"+
			"  const r = await Promise.all([num(5), num(6)]);\n"+
			"  console.log(r.join(','), r.length);\n"+
			"  const [f, o] = await Promise.all([num(7), obj(8)]);\n"+
			"  console.log(f, o.n);\n"+
			"  const ps: Promise<number>[] = [num(9), num(10)];\n"+
			"  console.log((await Promise.all(ps)).join('|'));\n"+
			"  const t: [number, string] = [1, 'a'];\n"+
			"  console.log(JSON.stringify(t), String(t), 0 in t, 5 in t);\n"+
			"}\n"+
			"main();\n")
	want := "2 x!\n" +
		"4 6 8\n" +
		"10,12 2\n" +
		"14 8\n" +
		"18|20\n" +
		"[1,\"a\"] 1,a true false\n"
	if got != want {
		t.Errorf("Promise.all over a tuple read differently than Node:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestPromiseAllOverATupleRejectsLikeNode pins the settling rule the tuple path shares
// with the array one: the combined promise rejects with the reason of the first input to
// reject, and does so without waiting on the rest. The inputs settle out of source order
// here so the answer cannot come from position alone.
func TestPromiseAllOverATupleRejectsLikeNode(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"function later<T>(v: T, ms: number): Promise<T> {\n"+
			"  return new Promise<T>((res) => { setTimeout(() => res(v), ms); });\n"+
			"}\n"+
			"function failLater(ms: number): Promise<string> {\n"+
			"  return new Promise<string>((res, rej) => { setTimeout(() => rej(new Error('boom')), ms); });\n"+
			"}\n"+
			"async function main() {\n"+
			"  const [a, b, c] = await Promise.all([later(1, 20), later('x', 5), later(true, 10)]);\n"+
			"  console.log(a, b, c);\n"+
			"  try {\n"+
			"    const [d, e] = await Promise.all([later(9, 30), failLater(5)]);\n"+
			"    console.log('unreachable', d, e);\n"+
			"  } catch (err) {\n"+
			"    console.log('caught', String(err));\n"+
			"  }\n"+
			"}\n"+
			"main();\n")
	want := "1 x true\n" +
		"caught Error: boom\n"
	if got != want {
		t.Errorf("Promise.all over a tuple settled differently than Node:\ngot:\n%s\nwant:\n%s", got, want)
	}
}
