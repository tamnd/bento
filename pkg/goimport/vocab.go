package goimport

// This file is the bento:go vocabulary module of document 16: the small library
// of TypeScript types that model Go concepts the language does not have natively.
// Every generated .d.ts imports the helpers it needs from "bento:go" (section
// 5.2), so this is the single source of truth for what those names mean. Keeping
// the vocabulary in one module is what makes the generated files small and
// consistent, and keeping it here, next to the Mapper that names the helpers,
// is what keeps the two from drifting: a test asserts every Helper the Mapper can
// emit has a declaration below.

// VocabularyModule is the specifier the generated declarations import from and the
// resolver serves this module for.
const VocabularyModule = "bento:go"

// Vocabulary returns the TypeScript declaration text of the bento:go module. It is
// the ambient declaration surface the checker reads so a generated .d.ts that
// imports GoReader or GoChannel type-checks, and it is the documentation of the
// marshaling contract each helper stands for (sections 6.8, 6.13, 7.7, 8, 10).
func Vocabulary() string { return vocabularyDTS }

const vocabularyDTS = `// The bento:go vocabulary: TypeScript types for Go concepts JavaScript lacks.
// Generated .d.ts files for go: imports draw their helper names from here.

// GoOpaque is a token for a Go value the bridge does not project: an option
// value, an unexported concrete type behind an interface, a struct with no
// exported surface. The author never inspects it; it is meaningful only when
// passed back into Go, which is exactly how such an API is meant to be used.
export type GoOpaque<Tag extends string> = { readonly __goOpaque: Tag };

// GoUnsupported marks a type that cannot cross the boundary at all, so reaching
// for a symbol that needs one is a legible checker error at the call site rather
// than a runtime surprise.
export type GoUnsupported = { readonly __goUnsupported: unique symbol };

// GoReader, GoWriter, and GoCloser are the projections of the common io
// interfaces. They are structural, so a TypeScript object with the right method
// satisfies one and can be handed to a Go API that wants the interface.
export interface GoReader {
  Read(p: Uint8Array): number;
}
export interface GoWriter {
  Write(p: Uint8Array): number;
}
export interface GoCloser {
  Close(): void;
}
export interface GoReadCloser extends GoReader, GoCloser {}
export interface GoWriteCloser extends GoWriter, GoCloser {}
export interface GoReadWriter extends GoReader, GoWriter {}

// GoError is a Go error surfaced to TypeScript: a normal Error whose message is
// the Go error string, carrying a handle to the original so Go's identity-based
// error handling stays usable through errors.Is and errors.As.
export declare class GoError extends Error {
  readonly message: string;
  readonly goError: GoOpaque<"error">;
  is(target: GoOpaque<"error">): boolean;
  as<T>(ctor: { readonly __goType: string }): T | null;
}

// GoChannel is a Go channel from TypeScript: both an async iterable, so a
// for-await-of consumes it until it closes, and a typed channel with explicit
// send, recv, and close for code that wants the operations directly.
export interface GoChannel<T> {
  [Symbol.asyncIterator](): AsyncIterator<T>;
  recv(): Promise<{ value: T; done: false } | { value: undefined; done: true }>;
  send(v: T): Promise<void>;
  close(): void;
}

// GoContext is an explicit Go context handle, for authors who port Go code or
// pass one context to several calls. The implicit AbortSignal/timeout options bag
// is the default; this is the opt-in.
export type GoContext = GoOpaque<"context.Context">;

// backgroundContext returns a context that is never canceled, the root a call
// graph derives its deadlines and cancellations from.
export declare function backgroundContext(): GoContext;

// withTimeout derives a context that cancels itself after ms milliseconds.
export declare function withTimeout(parent: GoContext, ms: number): GoContext;

// withCancel derives a context and the function that cancels it.
export declare function withCancel(parent: GoContext): [GoContext, () => void];

// withDeadline derives a context that cancels at an absolute epoch-millisecond
// deadline, and the function that cancels it early.
export declare function withDeadline(parent: GoContext, epochMs: number): [GoContext, () => void];

// select resolves the first ready channel operation, the faithful port of a Go
// select loop for code that needs its fairness across ready cases. Most authors
// reach for Promise.race over recv() and never need this.
export declare function select<T>(ops: Array<Promise<T>>): Promise<T>;
`
