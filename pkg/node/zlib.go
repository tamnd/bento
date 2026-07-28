package node

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"encoding/base64"
	"errors"
	"io"
)

// zlibResult is the JSON envelope the compression host function returns. It
// mirrors fsResult's shape: a success flag, the payload as base64, and on
// failure the code a program branches on plus the text zlib would have put in
// the message.
type zlibResult struct {
	OK   bool   `json:"ok"`
	B64  string `json:"b64,omitempty"`
	Code string `json:"code,omitempty"`
	Msg  string `json:"msg,omitempty"`
}

// The three wire formats Go's standard library covers, named as node:zlib names
// them. deflate is a zlib stream (RFC 1950), deflateRaw is a bare deflate
// stream (RFC 1951), and gzip is a gzip member (RFC 1952). unzip is not a
// format but inflate's sniffing mode: it accepts either of the first two
// framings and picks by the header.
const (
	formatDeflate    = "deflate"
	formatDeflateRaw = "deflateRaw"
	formatGzip       = "gzip"
	formatUnzip      = "unzip"
)

// zlibHostFuncs exposes one call for every codec. The direction, the framing and
// the level all ride as arguments rather than as separate functions, because the
// work either side of the compress/decompress split is the same marshalling.
func zlibHostFuncs() map[string]HostFunc {
	return map[string]HostFunc{
		"__bento_zlib_run": hostZlibRun,
	}
}

// hostZlibRun takes (format, direction, base64 input, level) and returns the
// envelope. Level is only read when compressing, and -1 asks for the library
// default.
func hostZlibRun(a []any) (any, error) {
	format := str(a, 0)
	direction := str(a, 1)
	data, err := base64.StdEncoding.DecodeString(str(a, 2))
	if err != nil {
		return zlibFail("Z_DATA_ERROR", "invalid input encoding"), nil
	}
	level := flate.DefaultCompression
	if len(a) > 3 && a[3] != nil {
		level = intArg(a, 3)
	}

	var out []byte
	if direction == "compress" {
		out, err = zlibCompress(format, data, level)
	} else {
		out, err = zlibDecompress(format, data)
	}
	if err != nil {
		return zlibFail(zlibCode(err), err.Error()), nil
	}
	return jsonString(zlibResult{OK: true, B64: base64.StdEncoding.EncodeToString(out)}), nil
}

func zlibCompress(format string, data []byte, level int) ([]byte, error) {
	// Go rejects any level outside [-2, 9]; zlib clamps instead, and Node passes
	// whatever the caller wrote straight through, so clamp here to keep a level of
	// 42 from turning into an error the C library would never raise.
	if level < flate.HuffmanOnly {
		level = flate.HuffmanOnly
	}
	if level > flate.BestCompression {
		level = flate.BestCompression
	}

	var buf bytes.Buffer
	var w io.WriteCloser
	var err error
	switch format {
	case formatDeflate:
		w, err = zlib.NewWriterLevel(&buf, level)
	case formatDeflateRaw:
		w, err = flate.NewWriter(&buf, level)
	case formatGzip:
		w, err = gzip.NewWriterLevel(&buf, level)
	default:
		return nil, errors.New("unknown compression format: " + format)
	}
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(data); err != nil {
		w.Close()
		return nil, err
	}
	// The trailer only lands on Close, so the buffer is not the stream until then.
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func zlibDecompress(format string, data []byte) ([]byte, error) {
	if format == formatUnzip {
		format = sniff(data)
	}
	// The header is checked before the stream is read so that input which is not
	// the framing at all is reported as a data error rather than as a truncated
	// one. Go's readers do not distinguish: a two-byte scrap and a genuine gzip
	// member cut short both come back as an unexpected EOF, and zlib calls only
	// the second of those Z_BUF_ERROR.
	if err := checkHeader(format, data); err != nil {
		return nil, err
	}
	switch format {
	case formatDeflate:
		return readAllClosing(func() (io.ReadCloser, error) { return zlib.NewReader(bytes.NewReader(data)) })
	case formatDeflateRaw:
		return readAllClosing(func() (io.ReadCloser, error) { return flate.NewReader(bytes.NewReader(data)), nil })
	case formatGzip:
		// Node's gunzip reads every member of a multi-member file, which is what
		// concatenated gzip output is; Go does the same once multistream is left on.
		return readAllClosing(func() (io.ReadCloser, error) { return gzip.NewReader(bytes.NewReader(data)) })
	default:
		return nil, errors.New("unknown compression format: " + format)
	}
}

// errBadHeader is the failure for input whose framing is wrong from the first
// bytes. zlib reports it as "incorrect header check", and reports it even for
// input too short to hold a whole header, which is why this is decided here
// rather than left to the reader.
var errBadHeader = errors.New("incorrect header check")

// checkHeader validates the fixed prefix of a framing. Raw deflate has no
// header to check, so nothing is rejected there.
func checkHeader(format string, data []byte) error {
	switch format {
	case formatGzip:
		if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
			return errBadHeader
		}
	case formatDeflate:
		// RFC 1950: the low nibble of the first byte is the compression method,
		// which is 8 for deflate, and the two header bytes read as a big-endian
		// number are a multiple of 31.
		if len(data) < 2 || data[0]&0x0f != 8 || (int(data[0])<<8|int(data[1]))%31 != 0 {
			return errBadHeader
		}
	}
	return nil
}

// sniff picks the framing for unzip. A gzip member starts with the fixed magic
// 1f 8b; a zlib stream starts with a compression-method-and-flags byte whose low
// nibble is 8 for deflate, and whose first two bytes are a multiple of 31. What
// matches neither is treated as a raw deflate stream, which is the only framing
// left.
func sniff(data []byte) string {
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		return formatGzip
	}
	if len(data) >= 2 && data[0]&0x0f == 8 && (int(data[0])<<8|int(data[1]))%31 == 0 {
		return formatDeflate
	}
	return formatDeflateRaw
}

// readAllClosing runs a reader to completion and closes it. The constructor is a
// callback rather than a reader because zlib and gzip validate their header when
// they are built, and that failure has to be reported like any other.
func readAllClosing(open func() (io.ReadCloser, error)) ([]byte, error) {
	r, err := open()
	if err != nil {
		return nil, err
	}
	out, err := io.ReadAll(r)
	closeErr := r.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return out, nil
}

// zlibCode maps a Go decompression failure onto the code node:zlib puts on the
// error. A stream that stops early is Z_BUF_ERROR, the way zlib reports running
// out of input; anything else malformed is Z_DATA_ERROR.
func zlibCode(err error) string {
	if errors.Is(err, errBadHeader) {
		return "Z_DATA_ERROR"
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return "Z_BUF_ERROR"
	}
	return "Z_DATA_ERROR"
}

func zlibFail(code, msg string) string {
	return jsonString(zlibResult{OK: false, Code: code, Msg: msg})
}
