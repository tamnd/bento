// ECMAScript breaks a line at four characters, \n \r U+2028 and U+2029, while RE2
// breaks one only at \n. An anchored multiline pattern therefore searches a subject
// whose other three terminators have been rewritten. Every character the rewrite
// replaces is one UTF-16 unit before and after, so the positions the language reports
// do not move; the byte offsets a captured substring is cut at do, which is what the
// mapping back exists for. The separators are written as escapes here because they are
// invisible in a source file.
function show(label: string, v: any): void { console.log(label + " => " + JSON.stringify(v)); }

const newlines: any = "alpha\nbeta\ngamma";
show("nl-first", /^(\w+)$/m.exec(newlines)?.[1]);
show("nl-middle", /^beta$/m.test(newlines));
show("nl-last", /^gamma$/m.test(newlines));
show("nl-miss", /^eta$/m.test(newlines));

const returns: any = "alpha\rbeta\rgamma";
show("cr-middle", /^beta$/m.test(returns));
show("cr-index", /^(beta)$/m.exec(returns)?.index);
show("cr-capture", /^(beta)$/m.exec(returns)?.[1]);

// The pair is two terminators, so the position between them ends one line and starts
// the next, and neither line's text carries either character.
const crlf: any = "a\r\nbb\r\nccc";
show("crlf-word", /^(\w+)$/m.exec(crlf)?.[1]);
show("crlf-bb", /^bb$/m.test(crlf));
show("crlf-ccc", /^ccc$/m.test(crlf));
show("crlf-empty", /^$/m.test(crlf));

const seps: any = "one\u2028two\u2029three";
show("ls-two", /^two$/m.test(seps));
show("ps-three", /^three$/m.test(seps));
show("sep-first", /^(\w+)$/m.exec(seps)?.[1]);
show("sep-index", /^(t\w+)$/m.exec(seps)?.index);
show("sep-capture", /^(t\w+)$/m.exec(seps)?.[1]);
show("sep-input", /^(t\w+)$/m.exec(seps)?.input);

const typedCrlf: string = "a\r\nbb\r\nccc";
show("replace-crlf", typedCrlf.replace(/^bb$/m, "Z"));
const typedSeps: string = "one\u2028two\u2029three";
show("replace-sep", typedSeps.replace(/^two$/m, "Z"));
show("replace-group", typedSeps.replace(/^(t\w+)$/m, "[$1]"));
show("search-crlf", typedCrlf.search(/^bb$/m));
show("search-sep", typedSeps.search(/^two$/m));

// A subject with no terminator is one line, so an anchor in the middle of it matches
// nothing. The rewrite must not invent a boundary.
const plain: string = "no breaks here";
show("plain-whole", /^no breaks here$/m.test(plain));
show("plain-inside", /^breaks$/m.test(plain));

show("dollar-before-cr", /a$/m.test("a\rb"));
show("dollar-before-sep", /a$/m.test("a\u2028b"));
show("unanchored-cr", /a\rb/m.test("a\rb"));

// The line the Node compat suite reads a Raspberry Pi's cpuinfo with, which is where
// this came from.
const hw: any = "processor : 0\nHardware\t: BCM2835\nRevision : 9000c1";
show("hardware", /^Hardware\s*:\s*(.*)$/im.exec(hw)?.[1]);
