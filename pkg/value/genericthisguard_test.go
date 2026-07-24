package value

import "testing"

// TestGenericMethodsThrowOnNullishThis proves every generic-receiver Array.prototype
// method takes the spec's ToObject(this value) step first: a null or undefined
// receiver has no object form, so ReturnIfAbrupt throws a TypeError before any length
// or index is read. A borrowed Array.prototype.<m>.call(null) therefore throws the way
// Node does rather than silently reading a zero length and returning.
func TestGenericMethodsThrowOnNullishThis(t *testing.T) {
	noop := NewFunc(func(args []Value) Value { return Undefined })
	reduce := NewFunc(func(args []Value) Value { return Arg(args, 0) })

	methods := []struct {
		name string
		run  func(recv Value)
	}{
		{"indexOf", func(r Value) { GenericIndexOf(r, Number(1)) }},
		{"lastIndexOf", func(r Value) { GenericLastIndexOf(r, Number(1)) }},
		{"fill", func(r Value) { GenericFill(r, Number(1)) }},
		{"join", func(r Value) { GenericJoin(r) }},
		{"copyWithin", func(r Value) { GenericCopyWithin(r) }},
		{"reverse", func(r Value) { GenericReverse(r) }},
		{"forEach", func(r Value) { GenericForEach(r, noop) }},
		{"slice", func(r Value) { GenericSlice(r) }},
		{"concat", func(r Value) { GenericConcat(r) }},
		{"map", func(r Value) { GenericMap(r, noop) }},
		{"filter", func(r Value) { GenericFilter(r, noop) }},
		{"some", func(r Value) { GenericSome(r, noop) }},
		{"every", func(r Value) { GenericEvery(r, noop) }},
		{"find", func(r Value) { GenericFind(r, noop) }},
		{"findIndex", func(r Value) { GenericFindIndex(r, noop) }},
		{"includes", func(r Value) { GenericIncludes(r, Number(1)) }},
		{"reduce", func(r Value) { GenericReduce(r, reduce) }},
		{"reduceRight", func(r Value) { GenericReduceRight(r, reduce) }},
	}

	for _, m := range methods {
		for _, recv := range []struct {
			label string
			val   Value
		}{{"null", Null}, {"undefined", Undefined}} {
			t.Run(m.name+"/"+recv.label, func(t *testing.T) {
				defer func() {
					if recover() == nil {
						t.Fatalf("Array.prototype.%s.call(%s) did not throw", m.name, recv.label)
					}
				}()
				m.run(recv.val)
			})
		}
	}
}
