package world

import (
	"encoding/binary"
	"fmt"
)

// Dimension identifies a Minecraft world dimension.
type Dimension int32

const (
	DimOverworld Dimension = 0
	DimNether    Dimension = 1
	DimEnd       Dimension = 2
)

// ChunkTag is the tag byte in a Bedrock LevelDB chunk key.
type ChunkTag byte

const (
	TagChunkVersion  ChunkTag = 0x76 // stored version of the chunk
	TagData2D        ChunkTag = 0x2D // height map + 2D biome IDs
	TagData2DLegacy  ChunkTag = 0x2E
	TagSubChunk      ChunkTag = 0x2F // terrain blocks (one entry per 16-tall section)
	TagLegacyTerrain ChunkTag = 0x30
	TagBlockEntity   ChunkTag = 0x31 // tile entities (chests, signs, …)
	TagEntity        ChunkTag = 0x32 // mobs and dropped items
	TagBiomeState    ChunkTag = 0x35 // 3D biomes (1.18+)
	TagFinalizedState ChunkTag = 0x36
)

// ChunkKey represents a parsed Bedrock LevelDB chunk key.
type ChunkKey struct {
	X, Z      int32
	Dimension Dimension
	Tag       ChunkTag
	SubY      int8 // subchunk Y index; only meaningful when Tag == TagSubChunk
}

func (ck ChunkKey) String() string {
	if ck.Tag == TagSubChunk {
		return fmt.Sprintf("chunk(%d,%d,dim=%d,subY=%d)", ck.X, ck.Z, ck.Dimension, ck.SubY)
	}
	return fmt.Sprintf("chunk(%d,%d,dim=%d,tag=0x%02X)", ck.X, ck.Z, ck.Dimension, ck.Tag)
}

// knownTags is the set of valid ChunkTag values. Used to reject non-chunk keys
// that happen to have a valid length (e.g. "~local_player" = 13 bytes).
var knownTags = map[byte]bool{
	byte(TagChunkVersion):   true,
	byte(TagData2D):         true,
	byte(TagData2DLegacy):   true,
	byte(TagSubChunk):       true,
	byte(TagLegacyTerrain):  true,
	byte(TagBlockEntity):    true,
	byte(TagEntity):         true,
	byte(TagBiomeState):     true,
	byte(TagFinalizedState): true,
	// Additional tags observed in the wild
	0x2B: true, // PendingTicks
	0x33: true, // PendingScheduledTicks
	0x34: true, // BorderBlocks
	0x39: true, // GenerationSeed
	0x3B: true, // CheckSums
	0x3C: true, // MetaDataHash
}

// ParseChunkKey parses a raw LevelDB key.
// Returns (key, true) if the key is a valid chunk key, or (zero, false) otherwise.
func ParseChunkKey(key []byte) (ChunkKey, bool) {
	n := len(key)
	// Valid lengths: 9, 10 (overworld), 13, 14 (other dimensions)
	if n != 9 && n != 10 && n != 13 && n != 14 {
		return ChunkKey{}, false
	}

	x := int32(binary.LittleEndian.Uint32(key[0:4]))
	z := int32(binary.LittleEndian.Uint32(key[4:8]))

	var dim Dimension
	var tagIdx int

	switch n {
	case 9, 10:
		dim = DimOverworld
		tagIdx = 8
	case 13, 14:
		rawDim := binary.LittleEndian.Uint32(key[8:12])
		dim = Dimension(rawDim)
		tagIdx = 12
	}

	tagByte := key[tagIdx]
	if !knownTags[tagByte] {
		return ChunkKey{}, false
	}
	tag := ChunkTag(tagByte)
	ck := ChunkKey{X: x, Z: z, Dimension: dim, Tag: tag}

	// SubChunk keys have one extra byte: the Y section index.
	if tag == TagSubChunk && tagIdx+1 < n {
		ck.SubY = int8(key[tagIdx+1])
	}

	return ck, true
}

// SubChunkAbsoluteY returns the lowest absolute Y coordinate for a subchunk.
func (ck ChunkKey) SubChunkAbsoluteY() int {
	return int(ck.SubY) * 16
}
