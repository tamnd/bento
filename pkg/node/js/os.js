// os implements node:os on top of a single Go host call that returns a snapshot
// of the platform data as JSON. Values that do not change during a run are read
// once; the ones that can move (free memory, load average, uptime) call through
// on each access.

__bento_defineModule("os", function (module, exports, require) {
  "use strict";

  const info = JSON.parse(__bento_os_info());
  const EOL = info.platform === "win32" ? "\r\n" : "\n";

  function fresh() {
    return JSON.parse(__bento_os_info());
  }

  module.exports = {
    EOL: EOL,
    platform: function () { return info.platform; },
    arch: function () { return info.arch; },
    type: function () { return info.type; },
    release: function () { return info.release; },
    version: function () { return info.version; },
    hostname: function () { return info.hostname; },
    homedir: function () { return info.homedir; },
    tmpdir: function () { return info.tmpdir; },
    endianness: function () { return info.endianness; },
    cpus: function () { return fresh().cpus; },
    totalmem: function () { return info.totalmem; },
    freemem: function () { return fresh().freemem; },
    uptime: function () { return fresh().uptime; },
    loadavg: function () { return fresh().loadavg; },
    networkInterfaces: function () { return fresh().networkInterfaces || {}; },
    userInfo: function () { return info.userInfo; },
    // availableParallelism counts the cores this process may run on, which is not
    // the length of the cpus array: that lists the cores the machine has, and a
    // process pinned to two cores of a machine with sixteen is told two here.
    availableParallelism: function () { return info.availableParallelism || 1; },
    // machine is what uname reports, which is not what arch reports: an x64
    // machine is "x86_64" to uname and "x64" to Node's arch. A platform whose
    // facts nothing measures yet answers the empty string rather than arch, which
    // would be a wrong answer dressed as a right one.
    machine: function () { return info.machine; },
    constants: { signals: {}, errno: {}, priority: {} },
    devNull: info.platform === "win32" ? "\\\\.\\nul" : "/dev/null",
  };
});
