package lower

import "testing"

// A key object with a Symbol.toPrimitive method is coerced to a property key with
// the string hint, the way ToPropertyKey routes ToPrimitive. The method's first
// declared parameter must receive the hint, not the receiver, so the emitted call
// passes the hint alone. This is the shape Object.fromEntries/to-property-key drives.
func TestSymbolToPrimitiveReceivesStringHint(t *testing.T) {
	skipIfShort(t)
	src := "var hintSeen: any = 'none';\nvar key: any = {[Symbol.toPrimitive]: function(hint: any){ hintSeen = hint; return 'key'; }};\nvar result: any = Object.fromEntries([[key, 'value']]);\nconsole.log('hint=' + String(hintSeen));\nconsole.log('val=' + String(result.key));\n"
	if got := runProgramGoTolerant(t, src); got != "hint=string\nval=value\n" {
		t.Fatalf("fromEntries to-property-key: got %q, want hint=string/val=value", got)
	}
}

// A template substitution coerces its expression with the string hint, read back
// by an exotic Symbol.toPrimitive that returns the hint name. This exercises the
// same string-hint path ToString and ToPropertyKey take, confirming the hint lands
// in the method's first parameter.
func TestSymbolToPrimitiveTemplateStringHint(t *testing.T) {
	skipIfShort(t)
	src := "var o: any = {[Symbol.toPrimitive]: function(hint: any){ return hint; }};\nconsole.log(String(`${o}`));\n"
	if got := runProgramGoTolerant(t, src); got != "string\n" {
		t.Fatalf("template string hint: got %q, want string", got)
	}
}
