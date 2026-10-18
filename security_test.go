// Copyright (c) 2026, go-avkit
// SPDX-License-Identifier: BSD-3-Clause

package vp9

import (
	"errors"
	"testing"
)

// superframeOf builds a packet of payload bytes followed by an index of
// four-byte sizes.
func superframeOf(payload int, sizes ...uint32) []byte {
	marker := byte(superframeMarker | 3<<3 | byte(len(sizes)-1))
	packet := make([]byte, payload, payload+2+4*len(sizes))
	packet = append(packet, marker)
	for _, s := range sizes {
		packet = append(packet, byte(s), byte(s>>8), byte(s>>16), byte(s>>24))
	}
	return append(packet, marker)
}

// ⛔ The sizes of a superframe index are four-byte values, and where int is 32
// bits wide one of 2^31 or more converts to a negative number. Sizes that sum
// to the payload only through such a number -- P+1 and then 0xFFFFFFFF, which
// is -1 -- passed the check that they account for the packet, and the second
// frame was then sliced backwards: a panic on arm, 386 and every other 32-bit
// target. On a 64-bit target the conversion is exact and this passes either
// way; the failure was reproduced under GOARCH=arm (linux/arm/v7).
func TestSuperframeSizeAboveTwoToTheThirtyOneIsRefused(t *testing.T) {
	for _, sizes := range [][]uint32{
		{17, 0xFFFFFFFF},
		{0x80000000, 0x80000010},
	} {
		packet := superframeOf(16, sizes...)
		if _, err := SplitSuperframe(packet); !errors.Is(err, ErrSuperframe) {
			t.Errorf("sizes %#x: err = %v, want ErrSuperframe", sizes, err)
		}
	}
}
