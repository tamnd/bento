// A tagged template is a call wearing a string's clothes. tag`a${x}b` invokes tag with
// the template's literal parts and its substitution values, and what the tag answers is
// the expression's value, which need not be a string at all.
//
// The first argument is the template strings object: an array of the cooked parts, with
// escapes resolved, carrying a raw property that holds the same parts exactly as the
// source spells them. Its identity belongs to the call site rather than to the call, so
// one template hands its tag the same frozen object however often it runs.
//
// Every tag here types its first parameter any, which is the slot the strings object
// lands in. A tag written in JavaScript gets that for nothing, since an unannotated
// parameter is any; a tag that annotates the parameter at a shape is a later slice.

// The plainest tag: fixed parameters, one per substitution.
function join3(parts: any, a: any, b: any): string {
  return parts[0] + "<" + a + ">" + parts[1] + "<" + b + ">" + parts[2];
}

// A rest parameter gathers however many substitutions the template has, which is how a
// tag is written when it does not know the shape of its call sites.
function gather(parts: any, ...vals: any[]): string {
  let out = String(parts[0]);
  for (let i = 0; i < vals.length; i++) {
    out += "{" + String(vals[i]) + "}" + String(parts[i + 1]);
  }
  return out;
}

// A tag reading the raw parts sees the escapes as written, so \n is a backslash and an
// n here while the cooked part beside it is a newline.
function showRaw(parts: any): string {
  return "cooked=" + JSON.stringify(parts[0]) + " raw=" + JSON.stringify(parts.raw[0]);
}

// A tag answering something that is not a string. The expression's value is whatever the
// tag returns, which is the whole reason the form is a call and not a template.
function count(parts: any, ...vals: any[]): number {
  return parts.length * 100 + vals.length;
}

// A tag that hands the splicing back to the language, spelled the way Node's own test
// harness spells it. String.raw reads the raw array off whatever object it is given, so
// what it splices here is the cooked parts, since that is what this object puts under
// raw. The tab below comes out as a tab for exactly that reason.
function viaRaw(parts: any, ...vals: any[]): string {
  return String.raw({ raw: parts }, ...vals);
}

const one = 1;
const two = "two";

console.log("join3", join3`p${one}q${two}r`);
console.log("gather-two", gather`p${one}q${two}r`);
console.log("gather-none", gather`plain`);
console.log("gather-adjacent", gather`${one}${two}`);
console.log("showRaw", showRaw`a\nb`);
console.log("count", count`p${one}q${two}r`);

// The built-in tag. It answers the raw text with the substitutions spliced in, which is
// what a Windows path or a regexp source wants: the escapes stay as they were typed.
console.log("String.raw", String.raw`c:\temp\new${one}`);
console.log("String.raw-plain", String.raw`\u0041 is not A here`);
console.log("viaRaw", viaRaw`x\ty${one}z`);

// String.raw is callable directly as well as through a template, and it reads raw off a
// plain object, so a hand-built one works the same way.
console.log("String.raw-call", String.raw({ raw: ["u", "v", "w"] }, 7, 8));

// The substitutions are evaluated left to right, once each, before the tag runs.
let order = "";
function note(s: string): string {
  order += s;
  return s;
}
console.log("order-result", gather`${note("a")}-${note("b")}`);
console.log("order", order);

// The strings object belongs to the site, not to the call, so the same template run
// twice hands the tag the same object, and a second site spelling the same text hands
// out a different one.
function identity(parts: any): any {
  return parts;
}
function sameSite(): any {
  return identity`shared`;
}
console.log("same-site", sameSite() === sameSite());
console.log("other-site", sameSite() === identity`shared`);

// The object is frozen, so a tag that writes to it changes nothing the next call sees.
function poke(parts: any): string {
  parts.mark = "written";
  return String(parts.mark);
}
console.log("frozen", poke`p`);

// A tag whose slot holds a box is called through the runtime rather than by a Go name,
// since there is no static function behind it to name. Both spellings go that way: a tag
// read out of a boxed object, and a bare binding that holds a boxed callable. A boxed
// method that reads this is a later slice, so the prefix here is closed over instead.
const prefix = "H:";
const holder: any = identity({
  tag(parts: any, v: any): string {
    return prefix + parts[0] + v + parts[1];
  },
});
console.log("boxed-tag", holder.tag`p${one}q`);

const bare: any = identity((parts: any, v: any): string => "B:" + parts[0] + v + parts[1]);
console.log("boxed-binding-tag", bare`p${two}q`);
