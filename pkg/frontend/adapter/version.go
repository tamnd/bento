package adapter

// PinnedRevision is the exact typescript-go commit bento's real adapter is built
// and tested against. Moving it is a deliberate act with a full frontend test
// run behind it, per 04_frontend_typescript_go.md section 3.
//
// It is unset while the real adapter is blocked: typescript-go keeps its
// checker, binder, and parser under internal/ as of mid-2026, so there is no
// package to import and nothing to pin yet. When the stable API lands (TS 7.1,
// or a bento fork that re-exports the internal packages through a public shim),
// this becomes the locked sha and Revision on the real adapter returns it.
const PinnedRevision = ""

// RealAdapterAvailable reports whether a real typescript-go-backed adapter can
// be constructed in this build. It is false until the upstream API is importable
// and PinnedRevision is locked. Load consults it to give a clear, honest error
// rather than a mysterious nil-checker crash.
func RealAdapterAvailable() bool {
	return PinnedRevision != ""
}
