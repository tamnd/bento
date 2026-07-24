package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestProxyRevocableAnyBindingStoresBox pins that an any binding does not trip the
// static-shape handback: Proxy.revocable stores the value.ProxyRevocable box straight
// through into the dynamic local. (Dynamically calling h.revoke() off that box is its
// own later slice; this test isolates the declaration the handback guards.)
func TestProxyRevocableAnyBindingStoresBox(t *testing.T) {
	const src = "export function f(): void { const h: any = Proxy.revocable([], {}); }\n"
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.ProxyRevocable(") {
		t.Fatalf("Proxy.revocable on an any binding did not lower to value.ProxyRevocable:\n%s", out)
	}
}

// TestProxyRevocableStructBindingHandsBack pins the boundary: a Proxy.revocable result
// captured under its concrete { proxy, revoke } object shape would build a Go struct
// the value.Value box cannot fill, so the emit read handle.Revoke off a value.Value and
// did not compile (the Array/isArray/proxy-revoked and concat is-concat-spreadable
// gobuild fails). The declaration now hands back with a named reason rather than emit
// that mismatch — a handback, never a wrong answer.
func TestProxyRevocableStructBindingHandsBack(t *testing.T) {
	const src = "const handle = Proxy.revocable([], {}); handle.revoke();\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "Proxy.revocable result bound under its") {
		t.Errorf("hand-back reason = %q, want it to mention the Proxy.revocable static shape", nyl.Reason)
	}
}
