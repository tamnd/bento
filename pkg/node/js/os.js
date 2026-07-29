// os implements node:os on top of a single Go host call that returns a snapshot
// of the platform data as JSON. Values that do not change during a run are read
// once; the ones that can move (free memory, load average, uptime, the processor
// times, the interface list) call through on each access.
//
// The members are defined in the order Node's own os module defines them, so
// Object.keys of this module reads the same as Object.keys of Node's. The compiled
// path builds the same module from the same measurements in pkg/value, and the two
// are pinned to that order on both sides.

__bento_defineModule("os", function (module, exports, require) {
  "use strict";

  const info = JSON.parse(__bento_os_info());
  const EOL = info.platform === "win32" ? "\r\n" : "\n";

  function fresh() {
    return JSON.parse(__bento_os_info());
  }

  module.exports = {
    arch: function () { return info.arch; },
    // availableParallelism counts the cores this process may run on, which is not
    // the length of the cpus array: that lists the cores the machine has, and a
    // process pinned to two cores of a machine with sixteen is told two here.
    availableParallelism: function () { return info.availableParallelism || 1; },
    cpus: function () { return fresh().cpus; },
    endianness: function () { return info.endianness; },
    freemem: function () { return fresh().freemem; },
    homedir: function () { return info.homedir; },
    hostname: function () { return info.hostname; },
    loadavg: function () { return fresh().loadavg; },
    networkInterfaces: function () { return fresh().networkInterfaces || {}; },
    platform: function () { return info.platform; },
    release: function () { return info.release; },
    tmpdir: function () { return info.tmpdir; },
    totalmem: function () { return info.totalmem; },
    type: function () { return info.type; },
    userInfo: function () { return info.userInfo; },
    uptime: function () { return fresh().uptime; },
    version: function () { return info.version; },
    // machine is what uname reports, which is not what arch reports: an x64
    // machine is "x86_64" to uname and "x64" to Node's arch. A platform whose
    // facts nothing measures yet answers the empty string rather than arch, which
    // would be a wrong answer dressed as a right one.
    machine: function () { return info.machine; },
    constants: { signals: {}, errno: {}, priority: {}, dlopen: {} },
    EOL: EOL,
    devNull: info.platform === "win32" ? "\\\\.\\nul" : "/dev/null",
  };
});
