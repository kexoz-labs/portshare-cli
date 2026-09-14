package tunnel

// wire.go — binary tunnel framing (protocol v2).
//
//	[4-byte big-endian metaLen][metaLen bytes JSON][raw body bytes]

import (
	"encoding/binary"
	"errors"
)

const maxTunnelMetaBytes = 1 << 20

var errInvalidTunnelFrame = errors.New("invalid tunnel frame")

type tunnelMeta struct {
	ID      string              `json:"id"`
	Method  string              `json:"method,omitempty"`
	Path    string              `json:"path,omitempty"`
	Headers map[string][]string `json:"headers,omitempty"`
	Status  int                 `json:"status,omitempty"`
	Error   string              `json:"error,omitempty"`
}

func encodeTunnelFrame(meta, body []byte) []byte {
	frame := make([]byte, 4+len(meta)+len(body))
	binary.BigEndian.PutUint32(frame[:4], uint32(len(meta)))
	copy(frame[4:], meta)
	copy(frame[4+len(meta):], body)
	return frame
}

func decodeTunnelFrame(data []byte) (meta, body []byte, err error) {
	if len(data) < 4 {
		return nil, nil, errInvalidTunnelFrame
	}
	metaLen := int(binary.BigEndian.Uint32(data[:4]))
	if metaLen < 0 || metaLen > maxTunnelMetaBytes || 4+metaLen > len(data) {
		return nil, nil, errInvalidTunnelFrame
	}
	return data[4 : 4+metaLen], data[4+metaLen:], nil
}
