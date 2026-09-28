// Copyright (c) 2026, go-avkit
// SPDX-License-Identifier: BSD-3-Clause

package vp9

import (
	"bytes"
	"errors"
	"testing"
)

// pack builds a superframe out of the frames given, with sizes of sizeBytes each.
func pack(sizeBytes int, frames ...[]byte) []byte {
	var out []byte
	for _, f := range frames {
		out = append(out, f...)
	}
	marker := byte(superframeMarker | (len(frames)-1)&0x7 | (sizeBytes-1)&0x3<<3)
	out = append(out, marker)
	for _, f := range frames {
		n := len(f)
		for i := 0; i < sizeBytes; i++ {
			out = append(out, byte(n>>(8*uint(i))))
		}
	}
	return append(out, marker)
}

func TestASuperframeIsSplitIntoItsFrames(t *testing.T) {
	for _, sizeBytes := range []int{1, 2, 3, 4} {
		a, b, c := []byte("first"), []byte("the second one"), []byte("third!")
		units, err := SplitSuperframe(pack(sizeBytes, a, b, c))
		if err != nil {
			t.Errorf("%d-byte sizes: %v", sizeBytes, err)
			continue
		}
		if len(units) != 3 {
			t.Errorf("%d-byte sizes: %d frames, want 3", sizeBytes, len(units))
			continue
		}
		for i, want := range [][]byte{a, b, c} {
			if !bytes.Equal(units[i], want) {
				t.Errorf("%d-byte sizes: frame %d = %q, want %q", sizeBytes, i+1, units[i], want)
			}
		}
	}
}

func TestAPlainPacketIsHandedBackWhole(t *testing.T) {
	packet := []byte{0x82, 0x49, 0x83, 0x42, 'x'}
	units, err := SplitSuperframe(packet)
	if err != nil {
		t.Fatalf("SplitSuperframe: %v", err)
	}
	if len(units) != 1 || !bytes.Equal(units[0], packet) {
		t.Errorf("%d frames: %v", len(units), units)
	}
}

// TestAByteThatMerelyLooksLikeAnIndexIsNotOne.
//
// ⛔ Three bits of pattern in one byte happen by chance, and an ordinary frame
// ending in such a byte is a frame. The whole reason the index states its length
// at BOTH ends and the sizes must account for exactly the bytes before it is so
// that a coincidence can be told from an index -- and a reader that trusted the
// pattern alone would cut an ordinary frame into pieces.
func TestAByteThatMerelyLooksLikeAnIndexIsNotOne(t *testing.T) {
	for _, tc := range []struct {
		name   string
		packet []byte
	}{
		{
			// The marker claims two frames of one byte, so the index is four
			// bytes, and the packet is shorter than that.
			"too short to hold the index it claims",
			[]byte{0xC1},
		},
		{
			// Long enough, but the byte where the index should start is not the
			// marker.
			"the two marker bytes disagree",
			[]byte{'a', 'b', 'c', 0x00, 2, 1, 0xC1},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			units, err := SplitSuperframe(tc.packet)
			if err != nil {
				t.Fatalf("SplitSuperframe: %v", err)
			}
			if len(units) != 1 || !bytes.Equal(units[0], tc.packet) {
				t.Errorf("an ordinary frame was cut into %d pieces", len(units))
			}
		})
	}
}

// TestAnIndexThatDoesNotDescribeItsPacketIsRefused: once the pattern, both marker
// bytes and the length all agree, this is an index and not a coincidence. Handing
// the packet back whole would present a decoder with the index bytes as frame
// data.
func TestAnIndexThatDoesNotDescribeItsPacketIsRefused(t *testing.T) {
	// Two frames of one byte claimed, three bytes of payload present.
	packet := []byte{'a', 'b', 'c', 0xC1, 1, 1, 0xC1}
	if _, err := SplitSuperframe(packet); !errors.Is(err, ErrSuperframe) {
		t.Errorf("err = %v, want ErrSuperframe", err)
	}
}

func TestAnEmptyFrameIsRefused(t *testing.T) {
	// One frame of zero bytes: the sizes do account for the payload, so this
	// passes the arithmetic and still describes nothing a decoder can read.
	packet := []byte{0xC0, 0, 0xC0}
	if _, err := SplitSuperframe(packet); !errors.Is(err, ErrSuperframe) {
		t.Errorf("err = %v, want ErrSuperframe", err)
	}
}

func TestAnEmptyPacketIsRefused(t *testing.T) {
	if _, err := SplitSuperframe(nil); !errors.Is(err, ErrSuperframe) {
		t.Errorf("err = %v, want ErrSuperframe", err)
	}
}

func TestSizesAreReadLeastSignificantFirst(t *testing.T) {
	// A size of 258 is 02 01 little-endian and 01 02 the other way round; only
	// one of those accounts for the payload.
	frame := bytes.Repeat([]byte{'x'}, 258)
	units, err := SplitSuperframe(pack(2, frame))
	if err != nil {
		t.Fatalf("SplitSuperframe: %v", err)
	}
	if len(units) != 1 || len(units[0]) != 258 {
		t.Errorf("%d frames, first of %d bytes, want one of 258", len(units), len(units[0]))
	}
}
