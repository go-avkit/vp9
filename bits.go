// Copyright (c) 2026, go-avkit
// SPDX-License-Identifier: BSD-3-Clause

package vp9

import "errors"

// ErrShort means the bitstream ended in the middle of a field.
var ErrShort = errors.New("vp9: bitstream ends inside a field")

// bitReader reads a VP9 header's fields, which are plain big-endian bit fields
// rather than the arithmetic coding the compressed part of a frame uses.
//
// It keeps the FIRST error and goes on returning zero, so a parse reads its
// fields in a straight line and is answered once at the end. That is not
// leniency: a zero read past the end is never handed to a caller, because err
// is checked before anything is returned. What it buys is that the shape of the
// header stays visible in the code -- the alternative, a check after every one
// of two dozen fields, hid the conditionals the format actually has among them.
type bitReader struct {
	data []byte
	pos  int // bits consumed
	err  error
}

// bit reads one bit, or zero once the reader has failed.
func (r *bitReader) bit() uint32 {
	if r.err != nil {
		return 0
	}
	if r.pos >= len(r.data)*8 {
		r.err = ErrShort
		return 0
	}
	b := r.data[r.pos/8]
	shift := 7 - uint(r.pos%8)
	r.pos++
	return uint32(b>>shift) & 1
}

// flag reads one bit as a flag.
func (r *bitReader) flag() bool { return r.bit() == 1 }

// literal reads n bits, most significant first.
func (r *bitReader) literal(n int) uint32 {
	var v uint32
	for i := 0; i < n; i++ {
		v = v<<1 | r.bit()
	}
	return v
}
