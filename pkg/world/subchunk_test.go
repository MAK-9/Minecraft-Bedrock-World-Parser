package world

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// buildSubChunkV8 constructs a minimal SubChunk v8 binary with a single storage layer.
//
// palette entries are encoded as little-endian NBT compounds.
// blockIndices maps linear index → palette index; positions absent default to 0.
func buildSubChunkV8(palette []BlockState, indices [4096]int) []byte {
	var buf bytes.Buffer

	// Header
	buf.WriteByte(8) // version
	buf.WriteByte(1) // layer count

	// Determine bitsPerBlock needed
	maxIdx := 0
	for _, v := range indices {
		if v > maxIdx {
			maxIdx = v
		}
	}
	bitsPerBlock := 1
	for (1 << bitsPerBlock) < len(palette) {
		bitsPerBlock++
	}
	if bitsPerBlock > 16 {
		bitsPerBlock = 16
	}

	// Storage header byte: (bitsPerBlock << 1) | 1 (persistent)
	buf.WriteByte(byte((bitsPerBlock<<1) | 1))

	// Pack block indices into uint32 words
	blocksPerWord := 32 / bitsPerBlock
	wordCount := (4096 + blocksPerWord - 1) / blocksPerWord
	mask := uint32((1 << bitsPerBlock) - 1)

	words := make([]uint32, wordCount)
	for i, idx := range indices {
		wordIdx := i / blocksPerWord
		bitPos := (i % blocksPerWord) * bitsPerBlock
		words[wordIdx] |= (uint32(idx) & mask) << bitPos
	}

	tmp := make([]byte, 4)
	for _, w := range words {
		binary.LittleEndian.PutUint32(tmp, w)
		buf.Write(tmp)
	}

	// Palette size
	binary.LittleEndian.PutUint32(tmp, uint32(len(palette)))
	buf.Write(tmp)

	// Palette NBT compounds
	for _, bs := range palette {
		buf.Write(encodeBlockStateNBT(bs))
	}

	return buf.Bytes()
}

// encodeBlockStateNBT encodes a BlockState as a Bedrock little-endian NBT compound.
func encodeBlockStateNBT(bs BlockState) []byte {
	var buf bytes.Buffer

	// TAG_Compound (0x0A) with empty name
	buf.WriteByte(0x0A)
	writeShortLE(&buf, 0)

	// TAG_String "name" → bs.Name
	buf.WriteByte(0x08)
	writeNBTStr(&buf, "name")
	writeNBTStr(&buf, bs.Name)

	// TAG_Compound "states" (empty for tests)
	buf.WriteByte(0x0A)
	writeNBTStr(&buf, "states")
	buf.WriteByte(0x00) // TAG_End

	// TAG_End (close outer compound)
	buf.WriteByte(0x00)

	return buf.Bytes()
}

func writeShortLE(buf *bytes.Buffer, v uint16) {
	tmp := make([]byte, 2)
	binary.LittleEndian.PutUint16(tmp, v)
	buf.Write(tmp)
}

func writeNBTStr(buf *bytes.Buffer, s string) {
	writeShortLE(buf, uint16(len(s)))
	buf.WriteString(s)
}

// --- Tests ---

func TestParseSubChunk_AllStone(t *testing.T) {
	palette := []BlockState{{Name: "minecraft:stone"}}
	var indices [4096]int // all 0 → all stone

	data := buildSubChunkV8(palette, indices)
	sc, err := ParseSubChunk(data)
	if err != nil {
		t.Fatalf("ParseSubChunk: %v", err)
	}

	for y := 0; y < 16; y++ {
		for z := 0; z < 16; z++ {
			for x := 0; x < 16; x++ {
				b := sc.Block(x, y, z)
				if b.Name != "minecraft:stone" {
					t.Fatalf("block(%d,%d,%d): got %q, want minecraft:stone", x, y, z, b.Name)
				}
			}
		}
	}
}

func TestParseSubChunk_MixedPalette(t *testing.T) {
	// palette: [0]=grass, [1]=stone
	// layer y=0 → grass (idx 0), everything else → stone (idx 1)
	palette := []BlockState{
		{Name: "minecraft:grass"},
		{Name: "minecraft:stone"},
	}
	var indices [4096]int
	// y=0: positions x+z*16+0*256 = x+z*16 ∈ [0, 255] → idx 0 (grass)
	// rest default to 0 (grass) too; set y=1..15 to 1 (stone)
	for y := 1; y < 16; y++ {
		for z := 0; z < 16; z++ {
			for x := 0; x < 16; x++ {
				indices[x+z*16+y*256] = 1
			}
		}
	}

	data := buildSubChunkV8(palette, indices)
	sc, err := ParseSubChunk(data)
	if err != nil {
		t.Fatalf("ParseSubChunk: %v", err)
	}

	// y=0 must be grass
	for z := 0; z < 16; z++ {
		for x := 0; x < 16; x++ {
			if b := sc.Block(x, 0, z); b.Name != "minecraft:grass" {
				t.Errorf("block(%d,0,%d): got %q, want minecraft:grass", x, z, b.Name)
			}
		}
	}

	// y=1 must be stone
	for z := 0; z < 16; z++ {
		for x := 0; x < 16; x++ {
			if b := sc.Block(x, 1, z); b.Name != "minecraft:stone" {
				t.Errorf("block(%d,1,%d): got %q, want minecraft:stone", x, z, b.Name)
			}
		}
	}
}

func TestParseSubChunk_TooShort(t *testing.T) {
	if _, err := ParseSubChunk([]byte{8}); err == nil {
		t.Error("expected error for too-short data")
	}
}

func TestParseSubChunk_UnsupportedVersion(t *testing.T) {
	if _, err := ParseSubChunk([]byte{3, 1}); err == nil {
		t.Error("expected error for unsupported version")
	}
}

func TestParseSubChunk_EmptyLayers(t *testing.T) {
	// layerCount = 0 should return all-air subchunk without error
	data := []byte{8, 0}
	sc, err := ParseSubChunk(data)
	if err != nil {
		t.Fatalf("ParseSubChunk empty: %v", err)
	}
	if b := sc.Block(0, 0, 0); !IsAir(b) {
		t.Errorf("expected air, got %q", b.Name)
	}
}

func TestIsAir(t *testing.T) {
	cases := []struct {
		b    BlockState
		want bool
	}{
		{BlockState{Name: "minecraft:air"}, true},
		{BlockState{Name: "air"}, true},
		{BlockState{Name: ""}, true},
		{BlockState{Name: "minecraft:stone"}, false},
		{BlockState{Name: "minecraft:grass"}, false},
	}
	for _, c := range cases {
		if got := IsAir(c.b); got != c.want {
			t.Errorf("IsAir(%q): got %v, want %v", c.b.Name, got, c.want)
		}
	}
}
