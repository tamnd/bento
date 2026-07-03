// Package cgodepfixture is a cgo-detection fixture for document 16 section 9.5. It
// is pure Go itself, with no C import of its own, but it imports runtime/cgo, a
// cgo package, so it stands for the shape the detector must catch: a library an
// author reaches through go: that is not cgo on its face but pulls a cgo package in
// transitively. The detector reports the runtime/cgo dependency, not this package,
// as the cgo library, which is what proves it walks the whole import graph rather
// than only looking at the named package.
package cgodepfixture

import _ "runtime/cgo"

// Marker is a plain function so the package holds ordinary pure-Go code the
// detector must not mistake for cgo; the cgo comes only from the import above.
func Marker() int { return 1 }
