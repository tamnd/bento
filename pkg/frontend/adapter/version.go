package adapter

// PinnedRevision is the exact typescript-go commit bento's real adapter is built
// and tested against. Moving it is a deliberate act with a full frontend test
// run behind it, per 04_frontend_typescript_go.md section 3.
//
// bento consumes typescript-go through the tamnd/typescript fork, which adds a
// public shim package over the compiler's internal checker, binder, and parser
// (upstream keeps them under internal/). This sha is that fork's commit, wired
// into go.mod with a replace directive; RealAdapter is built and tested against
// it, and Revision returns it.
const PinnedRevision = "1034b608e6e88fdb10575a7f6b1858538c5702af"

// RealAdapterAvailable reports whether a real typescript-go-backed adapter can
// be constructed in this build. It is false until the upstream API is importable
// and PinnedRevision is locked. Load consults it to give a clear, honest error
// rather than a mysterious nil-checker crash.
func RealAdapterAvailable() bool {
	return PinnedRevision != ""
}
