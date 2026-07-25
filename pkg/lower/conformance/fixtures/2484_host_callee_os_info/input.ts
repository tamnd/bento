// The os module reads its platform snapshot through the __bento_os_info host
// callee, a bento-internal global the AOT path resolves to a Go call into
// pkg/nodehost rather than the interpreter's host layer. The snapshot returns as
// a JSON string, so JSON.parse hands back an object whose fields carry the same
// kinds os.platform, os.arch, and os.userInfo report.
const info = JSON.parse(__bento_os_info());
console.log(typeof info.platform);
console.log(typeof info.arch);
console.log(typeof info.endianness);
console.log(typeof info.userInfo);
