package world

import (
	"encoding/binary"
	"testing"
)

func buildData2D(heights [256]int16, biomes [256]uint8) []byte {
	buf := make([]byte, Data2DSize)
	for i, h := range heights {
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(h))
	}
	copy(buf[512:], biomes[:])
	return buf
}

func TestParseData2D_Basic(t *testing.T) {
	var heights [256]int16
	var biomes [256]uint8
	for z := 0; z < 16; z++ {
		for x := 0; x < 16; x++ {
			heights[z*16+x] = int16(64 + x)
			biomes[z*16+x] = uint8(z)
		}
	}

	data := buildData2D(heights, biomes)
	d, err := ParseData2D(data)
	if err != nil {
		t.Fatalf("ParseData2D: %v", err)
	}

	if got := d.HeightAt(5, 3); got != 64+5 {
		t.Errorf("HeightAt(5,3): got %d, want %d", got, 64+5)
	}
	if got := d.BiomeAt(5, 3); got != 3 {
		t.Errorf("BiomeAt(5,3): got %d, want 3", got)
	}
}

func TestParseData2D_TooShort(t *testing.T) {
	if _, err := ParseData2D(make([]byte, 100)); err == nil {
		t.Error("expected error for short data")
	}
}

func TestParseData2D_ExtraBytes(t *testing.T) {
	var heights [256]int16
	var biomes [256]uint8
	data := append(buildData2D(heights, biomes), 0xFF, 0xFF) // extra bytes
	if _, err := ParseData2D(data); err != nil {
		t.Errorf("unexpected error with extra bytes: %v", err)
	}
}
