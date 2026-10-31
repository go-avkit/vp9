// Copyright (c) 2026, go-avkit
// SPDX-License-Identifier: BSD-3-Clause

package vp9

import (
	"errors"
	"fmt"
)

// ErrSuperframe means a packet carries an index that describes itself wrongly.
var ErrSuperframe = errors.New("vp9: superframe index is inconsistent")

// superframeMarker is the three high bits every index byte carries. The five
// low bits hold the frame count and the width of each size, both one less than
// they are.
const superframeMarker = 0xC0

// maxSuperframeFrames and maxSizeBytes are what those five bits can state.
const (
	maxSuperframeFrames = 8
	maxSizeBytes        = 4
)

// SplitSuperframe returns the frames a VP9 packet holds.
//
// A packet usually holds one frame and is handed back unchanged. It may instead
// be a superframe: several frames followed by an index that states how long each
// one is, which is how an encoder ships a frame nobody shows -- an alternate
// reference -- together with the frame that does show.
//
// ⛔ A packet whose last byte merely LOOKS like an index byte is handed back whole
// rather than refused. Three bits of pattern in one byte happen by chance, and an
// ordinary frame ending in such a byte is a frame, not a broken superframe: the
// index describes itself completely enough to say which it is, so the answer is
// taken from the arithmetic and not from the pattern. What the arithmetic
// requires is that the first and last bytes of the index agree and that the sizes
// account for exactly the bytes before it -- checked here, and the reason a
// coincidence cannot be mistaken for an index.
func SplitSuperframe(packet []byte) ([][]byte, error) {
	whole := [][]byte{packet}
	if len(packet) == 0 {
		return nil, fmt.Errorf("%w: the packet is empty", ErrSuperframe)
	}
	last := packet[len(packet)-1]
	if last&0xE0 != superframeMarker {
		return whole, nil
	}
	frames := int(last&0x7) + 1
	sizeBytes := int(last>>3&0x3) + 1
	index := 2 + frames*sizeBytes
	if len(packet) < index {
		return whole, nil
	}
	// The index states its own length twice, once at each end, so that a decoder
	// reading backwards and one reading forwards agree.
	if packet[len(packet)-index] != last {
		return whole, nil
	}

	// ⛔ Summed as the unsigned values they are. Converted to int first, a size
	// of 2^31 or more is negative where int is 32 bits wide, so sizes could sum
	// to the payload through a negative one -- P+1 and then 0xFFFFFFFF -- and
	// the frame after it was sliced backwards: a panic on every 32-bit target.
	// Eight sizes of at most 2^32-1 cannot overflow 64 bits, and once they
	// account for the payload exactly, each one fits an int.
	sizes := make([]uint32, frames)
	at := len(packet) - index + 1
	var total uint64
	for i := range sizes {
		sizes[i] = littleEndian(packet[at : at+sizeBytes])
		at += sizeBytes
		total += uint64(sizes[i])
	}
	payload := len(packet) - index
	if total != uint64(payload) {
		// Past this point the pattern, both marker bytes and the length all
		// agreed, so this is an index rather than a coincidence -- and it does
		// not describe the packet it sits in. Handing the packet back whole would
		// present a decoder with the index bytes as frame data.
		return nil, fmt.Errorf("%w: %d frames of %d bytes in all, but %d bytes precede the index",
			ErrSuperframe, frames, total, payload)
	}

	out := make([][]byte, 0, frames)
	off := 0
	for _, size := range sizes {
		if size == 0 {
			return nil, fmt.Errorf("%w: frame %d of %d is empty", ErrSuperframe, len(out)+1, frames)
		}
		n := int(size)
		out = append(out, packet[off:off+n])
		off += n
	}
	return out, nil
}

// littleEndian reads a size of one to four bytes, least significant first.
func littleEndian(b []byte) uint32 {
	var v uint32
	for i := len(b) - 1; i >= 0; i-- {
		v = v<<8 | uint32(b[i])
	}
	return v
}
