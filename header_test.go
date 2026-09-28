// Copyright (c) 2026, go-avkit
// SPDX-License-Identifier: BSD-3-Clause

package vp9

import (
	"errors"
	"testing"
)

// bitWriter builds a header bit by bit, so a test states what it means rather
// than a hex string nobody can check. It is the inverse of bitReader and exists
// only here: nothing in the package writes a bitstream.
type bitWriter struct {
	data []byte
	pos  int
}

func (w *bitWriter) bit(v uint32) {
	if w.pos%8 == 0 {
		w.data = append(w.data, 0)
	}
	if v&1 == 1 {
		w.data[w.pos/8] |= 1 << (7 - uint(w.pos%8))
	}
	w.pos++
}

func (w *bitWriter) literal(v uint32, n int) {
	for i := n - 1; i >= 0; i-- {
		w.bit(v >> uint(i) & 1)
	}
}

// keyFrame writes the uncompressed header of a profile-0 key frame of the given
// size, which is the shape almost every VP9 file in the wild begins with.
func keyFrame(width, height uint16) []byte {
	w := &bitWriter{}
	w.literal(frameMarker, 2)
	w.bit(0) // profile low
	w.bit(0) // profile high -> profile 0
	w.bit(0) // not show_existing_frame
	w.bit(0) // key frame
	w.bit(1) // shown
	w.bit(0) // not error resilient
	w.literal(syncCode, 24)
	w.literal(0, 3)                 // colour space unspecified
	w.bit(0)                        // studio range
	w.literal(uint32(width-1), 16)  //
	w.literal(uint32(height-1), 16) //
	w.bit(0)                        // render size is the coded size
	return w.data
}

func TestAKeyFrameStatesItsWholeDescription(t *testing.T) {
	h, err := ParseHeader(keyFrame(3840, 2160))
	if err != nil {
		t.Fatalf("ParseHeader: %v", err)
	}
	if !h.Key || !h.Show || h.ShowExisting || h.ErrorResilient {
		t.Errorf("flags: %+v", h)
	}
	if h.Profile != 0 || h.BitDepth != 8 {
		t.Errorf("profile %d, bit depth %d, want 0 and 8", h.Profile, h.BitDepth)
	}
	// ⛔ Profile 0 states no subsampling at all: it is 4:2:0 by definition. A
	// reader that expected two bits here would be misaligned for every frame of
	// the most common profile, and would still report a plausible size.
	if h.SubsamplingX != 1 || h.SubsamplingY != 1 {
		t.Errorf("subsampling %d,%d, want 1,1 implied by profile 0", h.SubsamplingX, h.SubsamplingY)
	}
	if h.Width != 3840 || h.Height != 2160 {
		t.Errorf("%dx%d, want 3840x2160", h.Width, h.Height)
	}
	if h.RenderWidth != 3840 || h.RenderHeight != 2160 {
		t.Errorf("render %dx%d, want the coded size", h.RenderWidth, h.RenderHeight)
	}
}

func TestASizeOfOneIsExpressibleAndZeroIsNot(t *testing.T) {
	// Both are stated one less than they are, so the smallest frame a stream can
	// state is 1x1 and no width of zero exists to be checked for.
	h, err := ParseHeader(keyFrame(1, 1))
	if err != nil {
		t.Fatalf("ParseHeader: %v", err)
	}
	if h.Width != 1 || h.Height != 1 {
		t.Errorf("%dx%d, want 1x1", h.Width, h.Height)
	}
}

func TestARenderSizeThatDiffersIsRead(t *testing.T) {
	w := &bitWriter{}
	w.literal(frameMarker, 2)
	w.literal(0, 2)
	w.bit(0)
	w.bit(0)
	w.bit(1)
	w.bit(0)
	w.literal(syncCode, 24)
	w.literal(0, 3)
	w.bit(0)
	w.literal(1919, 16)
	w.literal(1079, 16)
	w.bit(1) // a render size follows
	w.literal(1279, 16)
	w.literal(719, 16)

	h, err := ParseHeader(w.data)
	if err != nil {
		t.Fatalf("ParseHeader: %v", err)
	}
	if h.Width != 1920 || h.Height != 1080 {
		t.Errorf("coded %dx%d, want 1920x1080", h.Width, h.Height)
	}
	if h.RenderWidth != 1280 || h.RenderHeight != 720 {
		t.Errorf("render %dx%d, want 1280x720", h.RenderWidth, h.RenderHeight)
	}
}

func TestProfileTwoStatesItsBitDepth(t *testing.T) {
	for _, tc := range []struct {
		name string
		bit  uint32
		want uint8
	}{{"ten", 0, 10}, {"twelve", 1, 12}} {
		t.Run(tc.name, func(t *testing.T) {
			w := &bitWriter{}
			w.literal(frameMarker, 2)
			w.bit(0) // low
			w.bit(1) // high -> profile 2
			w.bit(0)
			w.bit(0)
			w.bit(1)
			w.bit(0)
			w.literal(syncCode, 24)
			w.bit(tc.bit) // the depth, which only profiles 2 and 3 state
			w.literal(0, 3)
			w.bit(0)
			w.literal(63, 16)
			w.literal(63, 16)
			w.bit(0)

			h, err := ParseHeader(w.data)
			if err != nil {
				t.Fatalf("ParseHeader: %v", err)
			}
			if h.Profile != 2 {
				t.Fatalf("profile %d, want 2", h.Profile)
			}
			if h.BitDepth != tc.want {
				t.Errorf("bit depth %d, want %d", h.BitDepth, tc.want)
			}
			if h.Width != 64 || h.Height != 64 {
				t.Errorf("%dx%d, want 64x64 -- the depth bit was misread if this is wrong",
					h.Width, h.Height)
			}
		})
	}
}

// TestAFrameThatOnlyRepeatsOneStatesNothingElse.
//
// ⛔ show_existing_frame ends the header there. A reader that went on would read
// the next frame's fields as this one's and hand back a description that belongs
// to nothing.
func TestAFrameThatOnlyRepeatsOneStatesNothingElse(t *testing.T) {
	w := &bitWriter{}
	w.literal(frameMarker, 2)
	w.literal(0, 2)
	w.bit(1)        // show_existing_frame
	w.literal(5, 3) // which one
	// Deliberate rubbish after it: nothing may read this.
	w.literal(0xFFFF, 16)

	h, err := ParseHeader(w.data)
	if err != nil {
		t.Fatalf("ParseHeader: %v", err)
	}
	if !h.ShowExisting || h.ExistingFrame != 5 {
		t.Errorf("%+v, want ShowExisting with frame 5", h)
	}
	if h.Width != 0 || h.BitDepth != 0 {
		t.Errorf("it described a frame it never read: %dx%d depth %d",
			h.Width, h.Height, h.BitDepth)
	}
}

func TestAnInterFrameTakesItsDescriptionFromElsewhere(t *testing.T) {
	w := &bitWriter{}
	w.literal(frameMarker, 2)
	w.literal(0, 2)
	w.bit(0)
	w.bit(1) // not a key frame
	w.bit(1)
	w.bit(0)

	h, err := ParseHeader(w.data)
	if err != nil {
		t.Fatalf("ParseHeader: %v", err)
	}
	if h.Key {
		t.Error("an inter frame reported itself as a key frame")
	}
	if h.Width != 0 {
		t.Errorf("width %d: an inter frame states none", h.Width)
	}
}

func TestHeaderRefusals(t *testing.T) {
	good := keyFrame(64, 64)

	t.Run("not VP9", func(t *testing.T) {
		bad := append([]byte(nil), good...)
		bad[0] &= 0x3F // clear the frame marker
		if _, err := ParseHeader(bad); !errors.Is(err, ErrNotVP9) {
			t.Errorf("err = %v, want ErrNotVP9", err)
		}
	})

	t.Run("wrong sync code", func(t *testing.T) {
		w := &bitWriter{}
		w.literal(frameMarker, 2)
		w.literal(0, 2)
		w.bit(0)
		w.bit(0)
		w.bit(1)
		w.bit(0)
		w.literal(syncCode^1, 24)
		if _, err := ParseHeader(w.data); !errors.Is(err, ErrSyncCode) {
			t.Errorf("err = %v, want ErrSyncCode", err)
		}
	})

	t.Run("4:4:4 in profile 0", func(t *testing.T) {
		w := &bitWriter{}
		w.literal(frameMarker, 2)
		w.literal(0, 2) // profile 0
		w.bit(0)
		w.bit(0)
		w.bit(1)
		w.bit(0)
		w.literal(syncCode, 24)
		w.literal(csSRGB, 3)
		w.literal(0, 40)
		if _, err := ParseHeader(w.data); !errors.Is(err, ErrProfile) {
			t.Errorf("err = %v, want ErrProfile", err)
		}
	})

	t.Run("4:2:0 in profile 1", func(t *testing.T) {
		w := &bitWriter{}
		w.literal(frameMarker, 2)
		w.bit(1) // low -> profile 1
		w.bit(0)
		w.bit(0)
		w.bit(0)
		w.bit(1)
		w.bit(0)
		w.literal(syncCode, 24)
		w.literal(0, 3)
		w.bit(0) // range
		w.bit(1) // subsampling x
		w.bit(1) // subsampling y -- 4:2:0, which profile 1 cannot state
		w.literal(0, 40)
		if _, err := ParseHeader(w.data); !errors.Is(err, ErrProfile) {
			t.Errorf("err = %v, want ErrProfile", err)
		}
	})

	t.Run("reserved bit after profile 3", func(t *testing.T) {
		w := &bitWriter{}
		w.literal(frameMarker, 2)
		w.bit(1) // low
		w.bit(1) // high -> profile 3
		w.bit(1) // the reserved bit, set
		if _, err := ParseHeader(w.data); !errors.Is(err, ErrReserved) {
			t.Errorf("err = %v, want ErrReserved", err)
		}
	})

	// ⛔ Every prefix of a good header must be refused, not completed from
	// nothing: a reader returning zeros past the end would turn a truncated
	// frame into a frame of a plausible size, which is indistinguishable from a
	// real one downstream.
	t.Run("every truncation", func(t *testing.T) {
		for n := 0; n < len(good); n++ {
			if _, err := ParseHeader(good[:n]); err == nil {
				t.Fatalf("%d bytes of a %d byte header parsed cleanly", n, len(good))
			}
		}
		if _, err := ParseHeader(good[:len(good)-1]); !errors.Is(err, ErrShort) {
			t.Errorf("err = %v, want ErrShort", err)
		}
	})
}

// profile1Subsampled writes a profile-1 key frame, which is the one shape that
// states its subsampling instead of implying it.
func profile1Subsampled() []byte {
	w := &bitWriter{}
	w.literal(frameMarker, 2)
	w.bit(1) // low -> profile 1
	w.bit(0)
	w.bit(0)
	w.bit(0)
	w.bit(1)
	w.bit(0)
	w.literal(syncCode, 24)
	w.literal(1, 3) // a colour space that is not sRGB
	w.bit(1)        // full range
	w.bit(1)        // subsampling x
	w.bit(0)        // subsampling y -- 4:2:2, which profile 1 may state
	w.bit(0)        // reserved
	w.literal(1279, 16)
	w.literal(719, 16)
	w.bit(0)
	return w.data
}

// profile1SRGB writes the other profile-1 shape: sRGB, where the range and the
// subsampling are implied rather than read.
func profile1SRGB() []byte {
	w := &bitWriter{}
	w.literal(frameMarker, 2)
	w.bit(1)
	w.bit(0)
	w.bit(0)
	w.bit(0)
	w.bit(1)
	w.bit(0)
	w.literal(syncCode, 24)
	w.literal(csSRGB, 3)
	w.bit(0) // reserved
	w.literal(63, 16)
	w.literal(63, 16)
	w.bit(0)
	return w.data
}

func TestProfileOneStatesItsSubsampling(t *testing.T) {
	h, err := ParseHeader(profile1Subsampled())
	if err != nil {
		t.Fatalf("ParseHeader: %v", err)
	}
	if h.Profile != 1 || !h.FullRange {
		t.Errorf("%+v", h)
	}
	if h.SubsamplingX != 1 || h.SubsamplingY != 0 {
		t.Errorf("subsampling %d,%d, want 1,0 as stated", h.SubsamplingX, h.SubsamplingY)
	}
	if h.Width != 1280 || h.Height != 720 {
		t.Errorf("%dx%d, want 1280x720 -- a misread subsampling bit shows up here",
			h.Width, h.Height)
	}
}

func TestSRGBImpliesFullRangeAndNoSubsampling(t *testing.T) {
	h, err := ParseHeader(profile1SRGB())
	if err != nil {
		t.Fatalf("ParseHeader: %v", err)
	}
	if !h.FullRange {
		t.Error("sRGB is full range by definition and was not reported so")
	}
	if h.SubsamplingX != 0 || h.SubsamplingY != 0 {
		t.Errorf("subsampling %d,%d, want 0,0: sRGB cannot be subsampled",
			h.SubsamplingX, h.SubsamplingY)
	}
	if h.Width != 64 || h.Height != 64 {
		t.Errorf("%dx%d, want 64x64", h.Width, h.Height)
	}
}

// TestAReservedBitSetAfterAColourSpaceIsRefused.
//
// ⛔ Each shape is built with the bit already set, rather than located in a
// finished buffer and flipped: a bit offset counted back from the end of a
// byte-padded buffer includes the padding, which is how the first version of
// this test passed for one shape and failed for the other.
func TestAReservedBitSetAfterAColourSpaceIsRefused(t *testing.T) {
	subsampled := func() []byte {
		w := &bitWriter{}
		w.literal(frameMarker, 2)
		w.bit(1) // profile 1
		w.bit(0)
		w.bit(0)
		w.bit(0)
		w.bit(1)
		w.bit(0)
		w.literal(syncCode, 24)
		w.literal(1, 3)
		w.bit(1) // range
		w.bit(1) // subsampling x
		w.bit(0) // subsampling y
		w.bit(1) // the reserved bit, set
		w.literal(0, 33)
		return w.data
	}
	srgb := func() []byte {
		w := &bitWriter{}
		w.literal(frameMarker, 2)
		w.bit(1) // profile 1
		w.bit(0)
		w.bit(0)
		w.bit(0)
		w.bit(1)
		w.bit(0)
		w.literal(syncCode, 24)
		w.literal(csSRGB, 3)
		w.bit(1) // the reserved bit, set
		w.literal(0, 33)
		return w.data
	}
	for name, build := range map[string]func() []byte{
		"subsampled": subsampled,
		"sRGB":       srgb,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseHeader(build()); !errors.Is(err, ErrReserved) {
				t.Errorf("err = %v, want ErrReserved", err)
			}
		})
	}
}

// renderSizeFrame writes a key frame that states a display size of its own, which
// is the only shape that reads the last four fields of the header.
func renderSizeFrame() []byte {
	w := &bitWriter{}
	w.literal(frameMarker, 2)
	w.literal(0, 2)
	w.bit(0)
	w.bit(0)
	w.bit(1)
	w.bit(0)
	w.literal(syncCode, 24)
	w.literal(0, 3)
	w.bit(0)
	w.literal(1919, 16)
	w.literal(1079, 16)
	w.bit(1)
	w.literal(1279, 16)
	w.literal(719, 16)
	return w.data
}

// TestEveryTruncationOfEveryShapeIsRefused.
//
// ⛔ The one property that matters most: no prefix of a header may parse. A
// reader that completed a field from nothing would report a frame size that looks
// real, and nothing downstream could tell it from one that was written. Each
// shape reads a different set of fields, so each is truncated at every byte.
func TestEveryTruncationOfEveryShapeIsRefused(t *testing.T) {
	shapes := map[string][]byte{
		"profile 0":            keyFrame(1920, 1080),
		"profile 1 subsampled": profile1Subsampled(),
		"profile 1 sRGB":       profile1SRGB(),
	}
	for name, whole := range shapes {
		t.Run(name, func(t *testing.T) {
			// The premise: the whole of it does parse, so a refusal below is
			// about the truncation and not about the fixture.
			if _, err := ParseHeader(whole); err != nil {
				t.Fatalf("the fixture does not parse: %v", err)
			}
			for n := 0; n < len(whole); n++ {
				if _, err := ParseHeader(whole[:n]); err == nil {
					t.Errorf("%d of %d bytes parsed cleanly", n, len(whole))
				}
			}
		})
	}
}
