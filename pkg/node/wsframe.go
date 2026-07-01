package node

import (
	"bufio"
	"crypto/rand"
	"encoding/binary"
	"io"
)

// This file implements just enough of RFC 6455 for the WebSocket client: reading
// frames off a hijacked connection and writing masked client frames back. It is
// deliberately small and dependency-free, in keeping with the pure-Go stack.

// WebSocket opcodes.
const (
	wsOpContinuation = 0x0
	wsOpText         = 0x1
	wsOpBinary       = 0x2
	wsOpClose        = 0x8
	wsOpPing         = 0x9
	wsOpPong         = 0xA
)

// wsFrame is one decoded frame. For control frames payload holds the control
// body; for data frames it holds that fragment's bytes.
type wsFrame struct {
	fin    bool
	opcode byte
	data   []byte
}

// readFrame decodes a single frame. Server-to-client frames are unmasked, but
// the code honors the mask bit anyway so it is robust against either peer.
func readFrame(r *bufio.Reader) (wsFrame, error) {
	var f wsFrame
	b0, err := r.ReadByte()
	if err != nil {
		return f, err
	}
	f.fin = b0&0x80 != 0
	f.opcode = b0 & 0x0F

	b1, err := r.ReadByte()
	if err != nil {
		return f, err
	}
	masked := b1&0x80 != 0
	length := uint64(b1 & 0x7F)
	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return f, err
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return f, err
		}
		length = binary.BigEndian.Uint64(ext[:])
	}

	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(r, maskKey[:]); err != nil {
			return f, err
		}
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return f, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}
	f.data = payload
	return f, nil
}

// writeFrame writes one masked client frame. Clients must mask every frame per
// the spec, so a fresh random mask key is generated for each write.
func writeFrame(w io.Writer, opcode byte, payload []byte) error {
	header := make([]byte, 0, 14)
	header = append(header, 0x80|opcode) // FIN set: bento sends unfragmented frames

	length := len(payload)
	switch {
	case length < 126:
		header = append(header, 0x80|byte(length))
	case length < 1<<16:
		header = append(header, 0x80|126)
		var ext [2]byte
		binary.BigEndian.PutUint16(ext[:], uint16(length))
		header = append(header, ext[:]...)
	default:
		header = append(header, 0x80|127)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(length))
		header = append(header, ext[:]...)
	}

	var maskKey [4]byte
	if _, err := rand.Read(maskKey[:]); err != nil {
		return err
	}
	header = append(header, maskKey[:]...)

	masked := make([]byte, length)
	for i := range payload {
		masked[i] = payload[i] ^ maskKey[i%4]
	}

	if _, err := w.Write(header); err != nil {
		return err
	}
	_, err := w.Write(masked)
	return err
}

// closePayload builds a close frame body from a code and reason, matching the
// RFC layout of a 2-byte big-endian code followed by the UTF-8 reason.
func closePayload(code int, reason string) []byte {
	if code == 0 {
		return nil
	}
	body := make([]byte, 2+len(reason))
	binary.BigEndian.PutUint16(body[:2], uint16(code))
	copy(body[2:], reason)
	return body
}

// parseClose extracts the code and reason from a close frame body. An empty body
// means no code was sent, which the spec treats as 1005.
func parseClose(body []byte) (int, string) {
	if len(body) < 2 {
		return 1005, ""
	}
	return int(binary.BigEndian.Uint16(body[:2])), string(body[2:])
}
