package lower

import (
	"strings"
	"testing"
)

// tagSrc is the preamble most cases here share. The tag's first parameter is typed any,
// which is the slot the template strings object lands in, and its rest gathers the
// substitutions, which is the shape a JavaScript tag takes without writing any of it
// down.
const tagSrc = "function tag(parts: any, ...vals: any[]): any { return parts; }\n"

// TestTaggedTemplateCallsTheTagWithTheStringsObject pins the shape of this slice: a
// tagged template is a call, and its first argument is the site's template strings
// object followed by the substitutions in source order.
func TestTaggedTemplateCallsTheTagWithTheStringsObject(t *testing.T) {
	src := tagSrc + "const a = 1;\nconst b = 2;\nconsole.log(tag`p${a}q${b}r`);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "Tag(tmplStrings1,") {
		t.Errorf("the tag was not called with the site's strings object:\n%s", source)
	}
	if !strings.Contains(source, "value.TemplateObject(") {
		t.Errorf("the strings object was not built through the runtime helper:\n%s", source)
	}
}

// TestTaggedTemplateHoistsOneObjectPerSite pins the identity rule. The language keys its
// template registry on the parse node, so the object is built once at init and every
// evaluation of that template hands the tag the same one, while a second site spelling
// the same text gets its own.
func TestTaggedTemplateHoistsOneObjectPerSite(t *testing.T) {
	src := tagSrc + "for (let i = 0; i < 2; i++) { console.log(tag`same`); }\nconsole.log(tag`same`);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "var tmplStrings1 = value.TemplateObject(") ||
		!strings.Contains(source, "var tmplStrings2 = value.TemplateObject(") {
		t.Errorf("two sites did not each reserve their own strings object:\n%s", source)
	}
	if strings.Contains(source, "tmplStrings3") {
		t.Errorf("a site reserved more than one strings object:\n%s", source)
	}
}

// TestTaggedTemplateCarriesCookedAndRawParts pins that both arrays go out, and that raw
// keeps the escape the cooked part resolved.
func TestTaggedTemplateCarriesCookedAndRawParts(t *testing.T) {
	src := tagSrc + "console.log(tag`a\\nb`);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, `value.FromGoString("a\nb")`) {
		t.Errorf("the cooked part did not resolve its escape:\n%s", source)
	}
	if !strings.Contains(source, `value.FromGoString("a\\nb")`) {
		t.Errorf("the raw part did not keep its escape as written:\n%s", source)
	}
}

// TestTaggedTemplateOnADynamicTagDispatchesThroughTheRuntime pins that a tag whose slot
// holds a box is called the way any dynamic callee is, rather than through a Go name
// that was never emitted for it.
func TestTaggedTemplateOnADynamicTagDispatchesThroughTheRuntime(t *testing.T) {
	src := "function take(x: any): any { return x; }\n" +
		"const t: any = take((parts: any, v: any): any => parts);\n" +
		"const a = 1;\nconsole.log(t`p${a}q`);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Call(tmplStrings1,") {
		t.Errorf("a boxed tag was not called through the runtime:\n%s", source)
	}
}

// TestStringRawTagCallsTheBuiltin pins the one tag the language ships. It needs no user
// function, so it goes straight to the runtime helper that splices the raw parts.
func TestStringRawTagCallsTheBuiltin(t *testing.T) {
	src := "const a = 1;\nconsole.log(String.raw`c:\\temp${a}`);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.StringRaw(tmplStrings1,") {
		t.Errorf("String.raw as a tag did not call the runtime helper:\n%s", source)
	}
}

// TestStringRawForwardsARestParameter pins the direct call with a spread, which is how a
// user tag hands its own substitutions back to the built-in. The count is a run-time
// fact, so the substitutions go as the array they already are.
func TestStringRawForwardsARestParameter(t *testing.T) {
	src := "function esc(parts: any, ...vals: any[]): string { return String.raw({ raw: parts }, ...vals); }\n" +
		"const a = 1;\nconsole.log(esc`x${a}y`);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.StringRawArgs(") {
		t.Errorf("a spread of the rest parameter did not reach the array-taking helper:\n%s", source)
	}
}

// TestTaggedTemplateWithAShapedStringsParameterHandsBack pins the honest refusal. A tag
// that annotates its first parameter at a shape rather than leaving it any asks for a Go
// slice, and the strings object is a box, which has no slot there.
func TestTaggedTemplateWithAShapedStringsParameterHandsBack(t *testing.T) {
	src := "function tag(parts: readonly string[], ...vals: any[]): string { return parts[0]; }\n" +
		"const a = 1;\nconsole.log(tag`p${a}q`);\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "template-strings parameter") {
		t.Errorf("the refusal did not name the template-strings parameter: %s", reason)
	}
}

// TestTaggedTemplateWithADefaultedParameterHandsBack pins the other refusal. Filling an
// omitted slot reads the declaration's defaults by position, and the leading strings
// object shifts every position by one, so the form is named rather than lowered onto the
// wrong slot.
func TestTaggedTemplateWithADefaultedParameterHandsBack(t *testing.T) {
	src := "function tag(parts: any, a: any = 5, b: any = 6): any { return a; }\n" +
		"const x = 1;\nconsole.log(tag`p${x}q`);\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "optional or defaulted parameter") {
		t.Errorf("the refusal did not name the defaulted parameter: %s", reason)
	}
}
