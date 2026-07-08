// A module-level object literal a top-level function reads hoists to a Go package
// var. The property names are fixed labels, not binding reads, so a literal whose
// values are constants carries no init-order dependency and hoists the way an
// array literal already does. A nested literal shows the whole tree hoists.

var config = { width: 4, height: 3, meta: { scale: 2 } };

function area() {
  return config.width * config.height * config.meta.scale;
}

console.log(area());

// An array of object literals, the common fixture-table shape, hoists too and
// reads back through a function.
var rows = [{ n: 10 }, { n: 20 }, { n: 30 }];

function total() {
  var s = 0;
  for (var i = 0; i < rows.length; i++) {
    s = s + rows[i].n;
  }
  return s;
}

console.log(total());
