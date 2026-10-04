# vp9

Pure-Go (CGO=0) reader for the parts of a VP9 bitstream that are **plain bit
fields**: the uncompressed frame header, and the superframe index that packs
several frames into one sample.

```go
frames, err := vp9.SplitSuperframe(packet)   // one sample may hold several
h, err := vp9.ParseHeader(frames[0])         // profile, size, show_frame, …
```

## What it is not

**It does not decode frames.** There is no boolean arithmetic coder, no
probability model, no residual, no prediction and no reconstruction — nothing
here turns a bitstream into pixels.

`ParseHeader` stops at the frame size on purpose, and the reason is in the
format: everything after it is either arithmetic-coded or of no use to a caller
describing a track. Stopping there keeps this readable against the one part of
VP9 that is plain bit fields.

The description on this repository used to say "bitstream reader **and
decoder**". It was wrong, and that matters more than it looks: a description is
what a search answers with, so a wrong one sends the next person to read the
code before they find out. The arithmetic coder the format needs lives in
[`go-avkit/boolcoder`](https://github.com/go-avkit/boolcoder).

## What is in it

| | |
|---|---|
| `SplitSuperframe` | the superframe index, so one sample yields its frames |
| `ParseHeader`, `Header` | the uncompressed header: profile, `show_existing_frame`, frame type, `show_frame`, colour, and both the coded and the render size |

Both the coded size and the render size are kept, because they differ: what a
frame is coded at is not always what it should be displayed at.

**Windows, macOS, Linux; six 64-bit architectures.** 100% statement coverage,
gated in CI.

## Licence

BSD-3-Clause.
