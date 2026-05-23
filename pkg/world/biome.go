package world

import (
	"encoding/binary"
	"fmt"
)

// Data2DSize is the size in bytes of a Bedrock Data2D value:
// 256 * uint16 (height map) + 256 * uint8 (biome IDs) = 768 bytes.
const Data2DSize = 256*2 + 256

// Data2D holds the per-column height map and 2D biome IDs for one chunk.
// Indexed by column = z*16 + x, where x, z ∈ [0, 16).
type Data2D struct {
	// HeightMap is the Y of the highest block in each column.
	HeightMap [256]int16
	// BiomeIDs maps each XZ column to its biome ID.
	BiomeIDs [256]uint8
}

// ParseData2D decodes the Data2D (tag 0x2D) value from LevelDB.
// The value must be at least 768 bytes; extra bytes are ignored.
func ParseData2D(data []byte) (*Data2D, error) {
	if len(data) < Data2DSize {
		return nil, fmt.Errorf("Data2D value too short: got %d bytes, need %d", len(data), Data2DSize)
	}

	d := &Data2D{}
	for i := 0; i < 256; i++ {
		d.HeightMap[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
	}
	copy(d.BiomeIDs[:], data[512:768])
	return d, nil
}

// BiomeAt returns the biome ID at local column (x, z) where x, z ∈ [0, 16).
func (d *Data2D) BiomeAt(x, z int) uint8 {
	return d.BiomeIDs[z*16+x]
}

// HeightAt returns the surface height at local column (x, z) where x, z ∈ [0, 16).
func (d *Data2D) HeightAt(x, z int) int16 {
	return d.HeightMap[z*16+x]
}
