package value

// This file gives the AOT runtime the DOM-style Event and EventTarget pair Node
// exposes as globals, the surface AbortSignal and several modules dispatch through.
// Both are backed by ordinary value objects carrying method closures rather than a
// Go struct, so a compiled program reads them through the same dynamic member and
// call path a require'd module object takes: the constructors return a value the
// caller holds as an any, and every method is a property whose closure shares the
// instance's private state.

// eventListener is one registration on an EventTarget: the boxed listener function
// and whether the registration removes itself after firing once. Registrations are
// held by pointer so a dispatch can identify the exact one to drop when it carries
// the once flag, and so removeEventListener can splice a match out by identity.
type eventListener struct {
	fn   Value
	once bool
}

// NewEvent builds an Event value of the given type, the object dispatchEvent hands
// each listener. It carries the read-only type and the bubbles and cancelable flags
// the init dictionary sets, and preventDefault marks it canceled only when it is
// cancelable, the spec's rule that a non-cancelable event ignores preventDefault.
// target and currentTarget start null and dispatch fills them, so a listener reading
// event.target sees the target it dispatched on. init is the optional second
// argument, read for its bubbles and cancelable members when it is an object.
func NewEvent(typeVal Value, init Value) Value {
	ev := NewObject()
	ev.Set(FromGoString("type"), StringValue(ToString(typeVal)))
	bubbles := false
	cancelable := false
	if init.kind == KindObject {
		bubbles = ToBoolean(init.Get(FromGoString("bubbles")))
		cancelable = ToBoolean(init.Get(FromGoString("cancelable")))
	}
	ev.Set(FromGoString("bubbles"), Bool(bubbles))
	ev.Set(FromGoString("cancelable"), Bool(cancelable))
	ev.Set(FromGoString("defaultPrevented"), False)
	ev.Set(FromGoString("target"), Null)
	ev.Set(FromGoString("currentTarget"), Null)
	ev.Set(FromGoString("preventDefault"), NewFunc(func(args []Value) Value {
		if cancelable {
			ev.Set(FromGoString("defaultPrevented"), True)
		}
		return Undefined
	}))
	ev.Set(FromGoString("stopPropagation"), NewFunc(func(args []Value) Value { return Undefined }))
	ev.Set(FromGoString("stopImmediatePropagation"), NewFunc(func(args []Value) Value { return Undefined }))
	return ev
}

// NewEventTarget builds an EventTarget value, the object addEventListener registers
// on and dispatchEvent fires. The registry maps an event type to its listeners in
// registration order, closed over by the three methods so they share one instance's
// state without a Go struct backing the value. Dispatch is single-target: with no
// node tree under a bare EventTarget there is no bubbling or capture to run, so a
// capture flag on a registration is accepted and has no effect, and the listeners
// for the dispatched type run in registration order.
func NewEventTarget() Value {
	et := NewObject()
	registry := map[string][]*eventListener{}

	et.Set(FromGoString("addEventListener"), NewFunc(func(args []Value) Value {
		typ := ToString(Arg(args, 0)).ToGoString()
		fn := Arg(args, 1)
		if fn.kind != KindFunc {
			return Undefined
		}
		once := false
		if opts := Arg(args, 2); opts.kind == KindObject {
			once = ToBoolean(opts.Get(FromGoString("once")))
		}
		// The spec ignores a second registration of the same listener for the same
		// type, so a duplicate does not fire twice; identity is the boxed function's.
		for _, l := range registry[typ] {
			if StrictEquals(l.fn, fn) {
				return Undefined
			}
		}
		registry[typ] = append(registry[typ], &eventListener{fn: fn, once: once})
		return Undefined
	}))

	et.Set(FromGoString("removeEventListener"), NewFunc(func(args []Value) Value {
		typ := ToString(Arg(args, 0)).ToGoString()
		fn := Arg(args, 1)
		ls := registry[typ]
		for i, l := range ls {
			if StrictEquals(l.fn, fn) {
				registry[typ] = append(ls[:i:i], ls[i+1:]...)
				return Undefined
			}
		}
		return Undefined
	}))

	et.Set(FromGoString("dispatchEvent"), NewFunc(func(args []Value) Value {
		ev := Arg(args, 0)
		typ := ToString(ev.Get(FromGoString("type"))).ToGoString()
		ev.Set(FromGoString("target"), et)
		ev.Set(FromGoString("currentTarget"), et)
		// Snapshot the list so a listener that adds or removes a listener during
		// dispatch does not change who runs this round, the spec's fixed set.
		snapshot := append([]*eventListener(nil), registry[typ]...)
		for _, l := range snapshot {
			if l.once {
				removeListener(registry, typ, l)
			}
			l.fn.Call(ev)
		}
		ev.Set(FromGoString("currentTarget"), Null)
		return Bool(!ToBoolean(ev.Get(FromGoString("defaultPrevented"))))
	}))

	return et
}

// removeListener splices one registration out of a type's list by pointer identity,
// the drop a once registration takes before it fires and the match removeEventListener
// finds. A registration not present is left as is, so a double removal is harmless.
func removeListener(registry map[string][]*eventListener, typ string, target *eventListener) {
	ls := registry[typ]
	for i, l := range ls {
		if l == target {
			registry[typ] = append(ls[:i:i], ls[i+1:]...)
			return
		}
	}
}
