// Copyright (c) 2026, go-avkit
// SPDX-License-Identifier: BSD-3-Clause

package vp9

import "errors"

// ErrShort means the bitstream ended in the middle of a field.
var ErrShort = errors.New("vp9: bitstream ends inside a field")

// bitReader reads a VP9 header's fields, which are plain big-endian bit fields
// rather than the arithmetic coding the compressed part of a frame uses.
//
// It counts what it has read rather than what remains, so a caller can say where
// a refusal happened: a header that ends one bit into a field is a different
// complaint from one that does not start at all, and both were met while this
// was written.
type bitReader struct {
	data []byte
	pos  int // bits consumed
}

// bit reads one bit. It reports ErrShort once and keeps reporting it: a reader
// that silently returned zeros past the end would turn a truncated header into a
// plausible one, which is the failure this whole package has to avoid.
func (r *bitReader) bit() (uint32, error) {
	if r.pos >= len(r.data)*8 {
		return 0, ErrShort
	}
	b := r.data[r.pos/8]
	shift := 7 - uint(r.pos%8)
	r.pos++
	return uint32(b>>shift) & 1, nil
}

// literal reads n bits, most significant first.
func (r *bitReader) literal(n int) (uint32, error) {
	var v uint32
	for i := 0; i < n; i++ {
		b, err := r.bit()
		if err != nil {
			return 0, err
		}
		v = v<<1 | b
	}
	return v, nil
}
