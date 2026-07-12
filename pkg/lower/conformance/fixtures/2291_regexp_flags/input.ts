// A RegExp reports its pattern through .source, its flag run through .flags in the
// specification's canonical order, and each flag through its own boolean getter. The
// literal below writes its flags out of order to show .flags reorders them, and the
// constructor form exercises the d and s flags the literal leaves unset.
const re = /ab+c/gi;
console.log(re.source);
console.log(re.flags);
console.log(re.global);
console.log(re.ignoreCase);
console.log(re.multiline);
console.log(re.dotAll);
console.log(re.sticky);
console.log(re.hasIndices);
const flagged = new RegExp("x.y", "ysd");
console.log(flagged.flags);
console.log(flagged.dotAll);
console.log(flagged.hasIndices);
console.log(flagged.global);
