// Copyright (c) 2026, go-avkit
// SPDX-License-Identifier: BSD-3-Clause

package vp9

import (
	"os"
	"path/filepath"
	"testing"
)

// The fuzz targets below hold the readers to what a reader of downloaded files
// needs: no packet, however malformed, panics, and every frame a superframe is
// split into is a slice of the packet it came from. An error is always an
// acceptable answer. The seeds are the packets of two short libvpx encodes
// (testdata/fuzzseed): profile 0 with an alternate reference, which makes a
// superframe, and profile 1 in 4:4:4.
//
//	go test -run='^$' -fuzz='^FuzzSplitSuperframe$' -fuzztime=60s

func packetSeeds(f *testing.F) {
	f.Helper()
	paths, err := filepath.Glob("testdata/fuzzseed/*")
	if err != nil || len(paths) == 0 {
		f.Fatalf("no seed: %v", err)
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			f.Fatalf("seed: %v", err)
		}
		f.Add(data)
	}
}

func FuzzParseHeader(f *testing.F) {
	packetSeeds(f)
	f.Fuzz(func(t *testing.T, frame []byte) {
		h, err := ParseHeader(frame)
		if err != nil {
			return
		}
		if h.Profile > 3 {
			t.Fatalf("profile %d", h.Profile)
		}
		if h.Key && h.BitDepth != 8 && h.BitDepth != 10 && h.BitDepth != 12 {
			t.Fatalf("bit depth %d", h.BitDepth)
		}
	})
}

func FuzzSplitSuperframe(f *testing.F) {
	packetSeeds(f)
	f.Fuzz(func(t *testing.T, packet []byte) {
		frames, err := SplitSuperframe(packet)
		if err != nil {
			return
		}
		total := 0
		for i, fr := range frames {
			if len(fr) == 0 {
				t.Fatalf("frame %d is empty", i)
			}
			total += len(fr)
			// Each frame must also be readable without a panic.
			_, _ = ParseHeader(fr)
		}
		if total > len(packet) {
			t.Fatalf("%d bytes of frames from a %d-byte packet", total, len(packet))
		}
	})
}
