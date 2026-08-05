package frontend

import (
	"strings"

	"github.com/tamnd/bento/pkg/cpath"
	"github.com/tamnd/bento/pkg/goimport"
)

// This file gives the AOT frontend the Node globals a real program uses. The
// TypeScript standard libraries declare Math, Number, String, and JSON, but not
// process or its streams, so a workload that reads process.env or writes
// process.stdout does not type-check under a stock strict configuration (the
// checker reports "Cannot find name 'process'"). bento does not want a caller to
// install @types/node to compile such a program, so it injects its own ambient
// declarations for the surface the compiler lowers, and no more: declaring only
// what lowers keeps the honest boundary, since a use of an undeclared global
// still errors rather than compiling to nothing.

// ambientPath is the virtual path of the synthetic library file bento adds to
// every AOT load. It is a .d.ts so its declarations are ambient globals, which is
// also what makes isAmbientGlobal treat process as library-provided rather than a
// user binding, the same test that separates the global Math from a local shadow.
const ambientPath = "/__bento_ambient__.d.ts"

// devolume drops a checker path's volume, so bento's synthetic paths are
// recognized by the one POSIX spelling they are written as here no matter which
// drive the program they belong to sits on. Every virtual path is handed to the
// checker through cpath.Virtual, which gives it the program's volume; this is the
// other half of that, and the two are what let the constants above stay readable
// instead of becoming per-load values.
func devolume(p string) string {
	if vol := cpath.Volume(p); vol != "" {
		return "/" + strings.TrimPrefix(p, vol)
	}
	return p
}

// isAmbientPath reports whether a path names the synthetic ambient library, on any
// volume.
func isAmbientPath(p string) bool { return devolume(p) == ambientPath }

// IsBentoAmbientPath reports whether a declaration's file is the synthetic library
// bento writes itself, as opposed to the TypeScript standard library the checker
// bundles. Both are .d.ts files, so a consumer that has to tell a global bento
// declares (process, module, require) from one it merely inherits (name, parseInt,
// Symbol) asks this rather than reading the path.
func IsBentoAmbientPath(p string) bool { return isAmbientPath(p) }

// ambientSource declares the Node globals and node: modules the AOT compiler can
// lower. process is typed any, the same shape module, exports, and require take,
// because the lowerer backs it with a real package-level process object
// (value.ProcessValue): a member read like process.platform or process.env.HOME
// lowers through the dynamic member path and resolves against that live object, so
// a name the object carries reads its value and one it does not reads undefined,
// exactly as in Node. An earlier revision declared a narrow BentoProcess interface
// instead, which typed the handful of members the lowerer special-cased; that made
// every other member a static "missing property" even though the runtime object
// has it, so the declaration follows the runtime rather than a fixed member list.
// The static process paths in the lowerer (process.on('exit'),
// process.stdout.write) still claim their call shapes before the dynamic path, so
// they keep emitting their direct helper calls. __dirname and __filename are the CommonJS module-path globals, each a
// string the lowerer fills from the module's own file path, so a program reading
// either resolves the same absolute path Node hands its wrapper. module and
// exports are the CommonJS export globals, typed any so a read of module.exports
// or a write of exports.x lowers through the dynamic member path; the lowerer
// backs them with a package-level module object. require is the CommonJS loader
// global, typed any so typeof require is "function" and a require(specifier) call
// lowers through the dynamic call path; the lowerer backs it with a package-level
// require function value. queueMicrotask is the WHATWG global that schedules a
// callback on the microtask queue; the lowerer boxes the callback and emits
// value.QueueMicrotask, and the assembled main drains the queue at its end.
// structuredClone is the WHATWG global that deep-copies a data graph; the lowerer
// boxes the argument and emits value.StructuredClone, whose clone the caller reads
// through the dynamic model. The four members added to the global Number interface,
// ref, unref, hasRef and refresh, are the Timeout object's methods: a scheduling call
// returns the timer's id here, which the standard library declares as a number, so the
// handle a program holds is a number and its methods have to be reachable on one. The
// declaration is wider than the truth, since it puts those names on every number, and
// it is the same width the lowerer already has: it turns one of those four calls on a
// number receiver into the matching timer operation whatever number it is. Typing them
// is what makes the result usable, so console.log(t.hasRef()) renders a boolean instead
// of refusing on a call the checker had no type for. setImmediate and clearImmediate are Node's own
// scheduling pair, declared here because the TypeScript standard library has only
// the WHATWG timers: the other four, setTimeout and setInterval with their clears,
// come from that library already and need no declaration. Each returns a number,
// the id its clear takes back, which is the shape the standard library's setTimeout
// declares and so the shape all six agree on. Event and EventTarget need no declaration here: the
// TypeScript standard library already declares the DOM-style event pair as ambient
// globals, so the lowerer recognizes a new Event or new EventTarget by name, builds
// the instance with value.NewEvent or value.NewEventTarget, and lands the binding in a
// value.Value slot read through the dynamic member and call path. __bento_os_info
// and __bento_inspect are host callees, the bare names a Node builtin factory
// reaches for when it needs the Go runtime: os.js reads __bento_os_info for the
// platform snapshot and util.js and assert.js read __bento_inspect to render a
// value, and url.js reads __bento_url_parse to resolve a URL against a base. Each
// is declared here so the checker types it, and the lowerer emits a direct call
// into the runtime (nodehost.OSInfoJSON, value.Inspect, nodehost.URLParseJSON)
// rather than the interpreter's host layer. The node:fs, node:os, and node:path module declarations give the file
// read and write surface a syscall workload uses without a caller installing
// @types/node, each function typed exactly as bento lowers it (readFileSync only
// in its encoding-and-string form, rmSync with the recursive and force options a
// benchmark passes). node:util declares the three members bento's util module
// carries, format, formatWithOptions and inspect, each typed with any for the
// values it renders, since rendering a value is the one thing util does that does
// not care what type it is. node:assert is declared differently from those four: its
// members are not lowered to named helpers but read off the module value the runtime
// registry builds, so every one of them is typed any and the module's default export
// is the callable module itself, which is what assert(true) calls. The four
// specifiers Node gives that module, with and without the node: scheme and in the
// strict form, are declared as one surface so a program written in any of them
// checks; which one a binding loads is decided by the specifier at the import, not
// here. The surface is deliberately small: it grows one entry
// at a time as the lowerer learns to emit each one, so a declared name is always a
// lowerable one.
const ambientSource = `declare var process: any;
declare var __dirname: string;
declare var __filename: string;
declare var module: any;
declare var exports: any;
declare var require: any;
declare function queueMicrotask(callback: () => void): void;
declare function setImmediate(callback: (...args: any[]) => void, ...args: any[]): number;
declare function clearImmediate(handle: number): void;
declare function structuredClone(value: any): any;
interface Number {
	ref(): number;
	unref(): number;
	hasRef(): boolean;
	refresh(): number;
}
declare function __bento_os_info(): string;
declare function __bento_inspect(value: any): string;
declare function __bento_url_parse(input: string, base: string): string;
declare module "node:fs" {
	export function mkdtempSync(prefix: string): string;
	export function writeFileSync(path: string, data: string): void;
	export function readFileSync(path: string, encoding: "utf8"): string;
	export function rmSync(path: string, options?: { recursive?: boolean; force?: boolean }): void;
}
declare module "node:os" {
	export function tmpdir(): string;
	export function platform(): string;
	export function arch(): string;
	export function type(): string;
	export function release(): string;
	export function version(): string;
	export function machine(): string;
	export function hostname(): string;
	export function homedir(): string;
	export function endianness(): string;
	export function totalmem(): number;
	export function freemem(): number;
	export function uptime(): number;
	export function availableParallelism(): number;
	export const EOL: string;
	export const devNull: string;
}
declare module "node:util" {
	export function format(format?: any, ...args: any[]): string;
	export function formatWithOptions(inspectOptions: any, format?: any, ...args: any[]): string;
	export function inspect(value: any, options?: any): string;
	export function isDeepStrictEqual(a: any, b: any): boolean;
}
declare module "node:assert" {
	const assert: any;
	export default assert;
	export const ok: any;
	export const fail: any;
	export const equal: any;
	export const notEqual: any;
	export const deepEqual: any;
	export const notDeepEqual: any;
	export const deepStrictEqual: any;
	export const notDeepStrictEqual: any;
	export const strictEqual: any;
	export const notStrictEqual: any;
	export const match: any;
	export const doesNotMatch: any;
	export const throws: any;
	export const doesNotThrow: any;
	export const ifError: any;
	export const strict: any;
}
declare module "assert" {
	export * from "node:assert";
	export { default } from "node:assert";
}
declare module "node:assert/strict" {
	export * from "node:assert";
	export { default } from "node:assert";
}
declare module "assert/strict" {
	export * from "node:assert";
	export { default } from "node:assert";
}
declare module "node:path" {
	export function join(...parts: string[]): string;
	export function resolve(...parts: string[]): string;
	export function normalize(p: string): string;
	export function dirname(p: string): string;
	export function basename(p: string, suffix?: string): string;
	export function extname(p: string): string;
	export function isAbsolute(p: string): boolean;
	export function relative(from: string, to: string): string;
	export function toNamespacedPath(p: string): string;
	export const sep: string;
	export const delimiter: string;
}
`

// ambientText is the full ambient library the overlay serves: the Node globals
// above followed by the bento:go vocabulary as a declare-module block, so a
// program that imports a go: package gets both without either living on disk. The
// vocabulary is owned by the goimport package, which is the single source of truth
// the generated .d.ts files draw their helper names from, so serving it from there
// keeps the declarations a generated file imports and the declarations the checker
// reads from ever drifting apart.
func ambientText() string {
	return ambientSource + goimport.AmbientModule()
}

// ambientOverlay serves the synthetic ambient library on top of a base
// FileSystem, so the checker reads bento's Node declarations without them living
// on disk. It overlays only the one virtual path; every other read falls through
// to the base, so a real project's files are untouched. It wraps the FS the host
// reads through and not the FS the resolver reads through, because the ambient
// file is a compile root rather than an imported module, so the resolver never
// looks for it and the base FS keeps its identity for the osFileSystem fast path.
type ambientOverlay struct{ base FileSystem }

func (a ambientOverlay) ReadFile(path string) (string, bool) {
	if isAmbientPath(path) {
		return ambientText(), true
	}
	return a.base.ReadFile(path)
}

func (a ambientOverlay) FileExists(path string) bool {
	if isAmbientPath(path) {
		return true
	}
	return a.base.FileExists(path)
}

func (a ambientOverlay) DirectoryExists(path string) bool {
	return a.base.DirectoryExists(path)
}
