package world

import (
	"encoding/binary"
	"testing"
)

func makeKey(x, z int32, dim *int32, tag byte, subY *byte) []byte {
	buf := make([]byte, 0, 14)
	tmp := make([]byte, 4)

	binary.LittleEndian.PutUint32(tmp, uint32(x))
	buf = append(buf, tmp...)
	binary.LittleEndian.PutUint32(tmp, uint32(z))
	buf = append(buf, tmp...)

	if dim != nil {
		binary.LittleEndian.PutUint32(tmp, uint32(*dim))
		buf = append(buf, tmp...)
	}

	buf = append(buf, tag)
	if subY != nil {
		buf = append(buf, *subY)
	}
	return buf
}

func TestParseChunkKey_Overworld(t *testing.T) {
	key := makeKey(1, -5, nil, byte(TagData2D), nil)
	ck, ok := ParseChunkKey(key)
	if !ok {
		t.Fatal("expected valid chunk key")
	}
	if ck.X != 1 || ck.Z != -5 {
		t.Errorf("coords: got (%d,%d), want (1,-5)", ck.X, ck.Z)
	}
	if ck.Dimension != DimOverworld {
		t.Errorf("dimension: got %d, want Overworld", ck.Dimension)
	}
	if ck.Tag != TagData2D {
		t.Errorf("tag: got 0x%02X, want 0x%02X", ck.Tag, TagData2D)
	}
}

func TestParseChunkKey_SubChunk(t *testing.T) {
	subY := byte(4)
	key := makeKey(1, -5, nil, byte(TagSubChunk), &subY)
	ck, ok := ParseChunkKey(key)
	if !ok {
		t.Fatal("expected valid chunk key")
	}
	if ck.Tag != TagSubChunk {
		t.Errorf("tag: got 0x%02X, want TagSubChunk", ck.Tag)
	}
	if ck.SubY != 4 {
		t.Errorf("SubY: got %d, want 4", ck.SubY)
	}
}

func TestParseChunkKey_Nether(t *testing.T) {
	dim := int32(DimNether)
	key := makeKey(3, 7, &dim, byte(TagBlockEntity), nil)
	ck, ok := ParseChunkKey(key)
	if !ok {
		t.Fatal("expected valid chunk key")
	}
	if ck.Dimension != DimNether {
		t.Errorf("dimension: got %d, want Nether", ck.Dimension)
	}
	if ck.X != 3 || ck.Z != 7 {
		t.Errorf("coords: got (%d,%d), want (3,7)", ck.X, ck.Z)
	}
}

func TestParseChunkKey_NetherSubChunk(t *testing.T) {
	dim := int32(DimNether)
	subY := byte(2)
	key := makeKey(0, 0, &dim, byte(TagSubChunk), &subY)
	ck, ok := ParseChunkKey(key)
	if !ok {
		t.Fatal("expected valid chunk key")
	}
	if ck.Dimension != DimNether {
		t.Errorf("dimension: got %d, want Nether", ck.Dimension)
	}
	if ck.Tag != TagSubChunk {
		t.Errorf("tag mismatch")
	}
	if ck.SubY != 2 {
		t.Errorf("SubY: got %d, want 2", ck.SubY)
	}
}

func TestParseChunkKey_InvalidLength(t *testing.T) {
	cases := [][]byte{
		{},
		{0x00, 0x01},
		make([]byte, 11),
		make([]byte, 15),
	}
	for _, key := range cases {
		if _, ok := ParseChunkKey(key); ok {
			t.Errorf("key len=%d: expected invalid, got valid", len(key))
		}
	}
}

func TestParseChunkKey_NonChunkKey(t *testing.T) {
	// e.g. "~local_player"
	key := []byte("~local_player")
	_, ok := ParseChunkKey(key)
	if ok {
		t.Error("expected non-chunk key to return false")
	}
}

func TestSubChunkAbsoluteY(t *testing.T) {
	subY := byte(3)
	key := makeKey(0, 0, nil, byte(TagSubChunk), &subY)
	ck, _ := ParseChunkKey(key)
	if got := ck.SubChunkAbsoluteY(); got != 48 {
		t.Errorf("AbsoluteY: got %d, want 48", got)
	}
}

func TestSubChunkAbsoluteY_Negative(t *testing.T) {
	// subY = -4 (as int8) means Y starts at -64 (post-1.18 worlds)
	subY := byte(0xFC) // -4 as int8
	key := makeKey(0, 0, nil, byte(TagSubChunk), &subY)
	ck, _ := ParseChunkKey(key)
	if got := ck.SubChunkAbsoluteY(); got != -64 {
		t.Errorf("AbsoluteY: got %d, want -64", got)
	}
}
