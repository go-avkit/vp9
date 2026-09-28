// Copyright (c) 2026, go-avkit
// SPDX-License-Identifier: BSD-3-Clause

// Package vp9 reads a VP9 bitstream in pure Go, with no libvpx or ffmpeg
// linkage and no external binaries.
//
// It begins with the uncompressed header, which is where VP9 states what a
// container does not: Matroska and MP4 carry no VP9 profile or level, so the
// frames are the only place either can be read from. That is why this exists
// before any decoding does -- a muxer needs the profile, the bit depth, the
// chroma subsampling and the frame size to write a usable vpcC record, and every
// one of them is in the first few dozen bits of a key frame.
package vp9

import (
	"errors"
	"fmt"
)

// Errors a header can be refused with.
var (
	// ErrNotVP9 means the frame does not begin with VP9's frame marker.
	ErrNotVP9 = errors.New("vp9: not a VP9 frame")
	// ErrSyncCode means a key frame's sync code is not the one VP9 states.
	ErrSyncCode = errors.New("vp9: wrong sync code")
	// ErrReserved means a bit the format reserves was set.
	ErrReserved = errors.New("vp9: reserved bit set")
	// ErrProfile means a profile and a colour description contradict each
	// other: 4:2:0 cannot be stated in profile 1 or 3, and 4:4:4 cannot be
	// stated in profile 0 or 2.
	ErrProfile = errors.New("vp9: colour description contradicts the profile")
)

// frameMarker is the two bits every VP9 frame begins with.
const frameMarker = 2

// syncCode is the 24 bits that follow a key frame's flags.
const syncCode = 0x498342

// csSRGB is the colour space value that means sRGB, the one case where a range
// and a subsampling are not read but implied.
const csSRGB = 7

// Header is what a VP9 frame's uncompressed header states.
//
// Only a key frame states the whole of it: a later frame either refers to a
// frame already decoded or repeats one, and takes its description from there. So
// a caller that needs a track's description reads the FIRST key frame, and Key
// says whether it got one.
type Header struct {
	Profile        uint8 // 0 to 3
	Key            bool  // A key frame states its own description.
	Show           bool  // The frame is shown rather than only decoded.
	ErrorResilient bool
	ShowExisting   bool  // The frame only repeats one already decoded.
	ExistingFrame  uint8 // Which one, when ShowExisting.
	BitDepth       uint8 // 8, 10 or 12
	ColorSpace     uint8 // 0 to 7, as ISO/IEC 23001-8 numbers them
	FullRange      bool
	SubsamplingX   uint8 // 1 means the chroma planes are half as wide
	SubsamplingY   uint8
	Width          uint16
	Height         uint16
	RenderWidth    uint16 // The size to display at, which may differ.
	RenderHeight   uint16
}

// ParseHeader reads the uncompressed header at the start of one VP9 frame.
//
// It reads no further than it needs: everything after the frame size is either
// arithmetic-coded or of no use to a caller describing a track, and stopping
// here keeps this readable against the one part of the format that is plain bit
// fields.
func ParseHeader(frame []byte) (Header, error) {
	r := &bitReader{data: frame}
	var h Header

	marker := r.literal(2)
	// ⛔ Checked before the comparison, and again after the sync code below, for
	// one reason: a reader that has run out returns zero, and a zero compared
	// against the marker would answer "not VP9" for a frame that was merely cut.
	// Those are different complaints and a caller acts differently on each.
	if r.err != nil {
		return h, r.err
	}
	if marker != frameMarker {
		return h, fmt.Errorf("%w: frame marker is %d, not %d", ErrNotVP9, marker, frameMarker)
	}
	// The profile arrives low bit first, and a value above 2 spends one more bit
	// that the format reserves.
	low, high := r.bit(), r.bit()
	h.Profile = uint8(low | high<<1)
	if h.Profile > 2 && r.bit() != 0 {
		return h, fmt.Errorf("%w: after profile %d", ErrReserved, h.Profile)
	}

	if r.bit() == 1 {
		// A frame that only repeats one already decoded states which, and
		// nothing else: there is no description to read here, and a caller
		// after one has to look at another frame.
		h.ShowExisting, h.Show = true, true
		h.ExistingFrame = uint8(r.literal(3))
		return h, r.err
	}

	h.Key = r.bit() == 0
	h.Show = r.flag()
	h.ErrorResilient = r.flag()
	if !h.Key {
		// An inter frame's description comes from the frames it refers to, and
		// reading those means keeping state this does not yet keep. What it
		// stated so far is still worth handing back.
		return h, r.err
	}

	sync := r.literal(24)
	if r.err != nil {
		return h, r.err
	}
	if sync != syncCode {
		return h, fmt.Errorf("%w: %#06x, not %#06x", ErrSyncCode, sync, syncCode)
	}
	if err := h.readColour(r); err != nil {
		return h, err
	}
	h.readSize(r)
	return h, r.err
}

// readColour reads the bit depth, colour space, range and subsampling.
//
// ⛔ The subsampling is read from the stream in profiles 1 and 3 and IMPLIED in 0
// and 2, and which way round depends on the colour space as well. A reader that
// took the same bits in every profile would be misaligned from here on for every
// profile-0 frame, which is most of them, and would still produce plausible
// numbers -- so the profile is checked against the colour space and the
// contradiction is an error rather than a silent reinterpretation.
//
// The contradictions are judged on values that a cut stream reads as zero, and
// zero contradicts nothing: the refusals here cannot fire on a truncation, and
// the reader's own error is what answers that.
func (h *Header) readColour(r *bitReader) error {
	h.BitDepth = 8
	if h.Profile >= 2 {
		h.BitDepth = 10
		if r.flag() {
			h.BitDepth = 12
		}
	}
	h.ColorSpace = uint8(r.literal(3))
	highProfile := h.Profile == 1 || h.Profile == 3

	if h.ColorSpace == csSRGB {
		// sRGB is full range by definition and cannot be subsampled.
		h.FullRange = true
		if !highProfile {
			return fmt.Errorf("%w: 4:4:4 needs profile 1 or 3, not %d", ErrProfile, h.Profile)
		}
		if r.bit() != 0 {
			return fmt.Errorf("%w: after an sRGB colour space", ErrReserved)
		}
		return nil
	}

	h.FullRange = r.flag()
	if !highProfile {
		// Profiles 0 and 2 are 4:2:0 and state nothing.
		h.SubsamplingX, h.SubsamplingY = 1, 1
		return nil
	}
	h.SubsamplingX, h.SubsamplingY = uint8(r.bit()), uint8(r.bit())
	if h.SubsamplingX == 1 && h.SubsamplingY == 1 {
		return fmt.Errorf("%w: 4:2:0 cannot be stated in profile %d", ErrProfile, h.Profile)
	}
	if r.bit() != 0 {
		return fmt.Errorf("%w: after a subsampled colour space", ErrReserved)
	}
	return nil
}

// readSize reads the coded frame size and, when the stream states one, the size
// it is meant to be displayed at.
//
// Both are stated one less than they are, so a zero width is not expressible and
// no check for one is needed. A truncation is the reader's to report, so this
// returns nothing.
func (h *Header) readSize(r *bitReader) {
	h.Width, h.Height = uint16(r.literal(16))+1, uint16(r.literal(16))+1
	h.RenderWidth, h.RenderHeight = h.Width, h.Height
	if !r.flag() {
		return
	}
	h.RenderWidth, h.RenderHeight = uint16(r.literal(16))+1, uint16(r.literal(16))+1
}
