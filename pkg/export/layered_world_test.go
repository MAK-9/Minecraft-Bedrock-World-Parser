package export_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/mak-9/minecraft-bedrock-world-parser/pkg/export"
	"github.com/mak-9/minecraft-bedrock-world-parser/pkg/world"
	_ "modernc.org/sqlite"
)

// ---------------------------------------------------------------------------
// Layer definitions
// ---------------------------------------------------------------------------

// The fake world has one 16×16 chunk, one sub-chunk (SubY=0, Y 0-15).
//
// Block layout from bottom to top:
//   Y=0      minecraft:bedrock    (1 layer)
//   Y=1-3    minecraft:deepslate  (3 layers)
//   Y=4-6    minecraft:stone      (3 layers)
//   Y=7-9    minecraft:dirt       (3 layers)
//   Y=10     minecraft:grass_block (1 layer — the surface)
//   Y=11-15  minecraft:air

const (
	piAir       = 0
	piBedrock   = 1
	piDeepslate = 2
	piStone     = 3
	piDirt      = 4
	piGrass     = 5
)

var testPalette = []string{
	"minecraft:air",
	"minecraft:bedrock",
	"minecraft:deepslate",
	"minecraft:stone",
	"minecraft:dirt",
	"minecraft:grass_block",
}

// paletteIndexAt returns the palette index for blocks at the given local Y.
func paletteIndexAt(y int) int {
	switch {
	case y == 0:
		return piBedrock
	case y <= 3:
		return piDeepslate
	case y <= 6:
		return piStone
	case y <= 9:
		return piDirt
	case y == 10:
		return piGrass
	default:
		return piAir
	}
}

// expectedBlockAt returns the block name the parser should report for local Y.
func expectedBlockAt(y int) string {
	return testPalette[paletteIndexAt(y)]
}

// ---------------------------------------------------------------------------
// Binary sub-chunk builder
// ---------------------------------------------------------------------------

// buildSubChunkData returns valid Bedrock v8 sub-chunk binary data for the
// layered world described above.  Each Y slice (16×16 = 256 positions) is
// filled uniformly with the block for that layer.
//
// Storage format:
//   [0]        version = 8
//   [1]        layerCount = 1
//   [2]        blockStorageHeader = (bitsPerBlock<<1)|1  (bitsPerBlock=4 → 0x09)
//   [3..2050]  512 packed uint32 words (8 blocks per word, 4 bits each)
//   [2051..2054] palette entry count as uint32 LE
//   [2055..]   NBT palette entries
func buildSubChunkData() []byte {
	const bitsPerBlock = 4 // fits 6-entry palette; 8 blocks per word
	const blocksPerWord = 32 / bitsPerBlock // 8
	const wordCount = (4096 + blocksPerWord - 1) / blocksPerWord // 512

	// Build the 4096-entry index array in XZY order (x + z*16 + y*256).
	var indices [4096]uint32
	for y := 0; y < 16; y++ {
		pi := uint32(paletteIndexAt(y))
		base := y * 256
		for i := 0; i < 256; i++ {
			indices[base+i] = pi
		}
	}

	// Pack indices into uint32 words.
	words := make([]uint32, wordCount)
	for i, pi := range indices {
		words[i/blocksPerWord] |= pi << uint((i%blocksPerWord)*bitsPerBlock)
	}

	var buf bytes.Buffer

	// Sub-chunk header
	buf.WriteByte(8) // version
	buf.WriteByte(1) // layerCount

	// Block storage header
	buf.WriteByte(byte((bitsPerBlock << 1) | 1)) // 0x09

	// Packed words (little-endian)
	wb := make([]byte, 4)
	for _, w := range words {
		binary.LittleEndian.PutUint32(wb, w)
		buf.Write(wb)
	}

	// Palette size
	binary.LittleEndian.PutUint32(wb, uint32(len(testPalette)))
	buf.Write(wb)

	// Palette entries as little-endian NBT compounds
	for _, name := range testPalette {
		writeNBTPaletteEntry(&buf, name)
	}

	return buf.Bytes()
}

// writeNBTPaletteEntry encodes one Bedrock block-state NBT compound.
// Structure (little-endian NBT):
//   TAG_Compound + "" (root)
//     TAG_String  "name"    = blockName
//     TAG_Compound "states" = {} (empty)
//     TAG_Int     "version" = 17959425
//   TAG_End
func writeNBTPaletteEntry(w *bytes.Buffer, blockName string) {
	w.WriteByte(0x0A) // TAG_Compound
	nbtString(w, "") // root name (empty)

	// "name" field
	w.WriteByte(0x08) // TAG_String
	nbtString(w, "name")
	nbtString(w, blockName)

	// "states" field (empty compound)
	w.WriteByte(0x0A) // TAG_Compound
	nbtString(w, "states")
	w.WriteByte(0x00) // TAG_End for states

	// "version" field
	w.WriteByte(0x03) // TAG_Int
	nbtString(w, "version")
	vb := make([]byte, 4)
	binary.LittleEndian.PutUint32(vb, 17959425)
	w.Write(vb)

	w.WriteByte(0x00) // TAG_End for compound
}

func nbtString(w *bytes.Buffer, s string) {
	lb := make([]byte, 2)
	binary.LittleEndian.PutUint16(lb, uint16(len(s)))
	w.Write(lb)
	w.WriteString(s)
}

// ---------------------------------------------------------------------------
// Helper: parse once, reuse in all tests
// ---------------------------------------------------------------------------

func parseLayeredSubChunk(t *testing.T) *world.SubChunk {
	t.Helper()
	sc, err := world.ParseSubChunk(buildSubChunkData())
	if err != nil {
		t.Fatalf("ParseSubChunk: %v", err)
	}
	return sc
}

func layeredChunkData(sc *world.SubChunk) world.ChunkData {
	return world.ChunkData{
		Key: world.ChunkKey{X: 0, Z: 0, Dimension: world.DimOverworld},
		SubChunks: map[int8]*world.SubChunk{
			int8(0): sc,
		},
	}
}

// ---------------------------------------------------------------------------
// Test 1: sub-chunk parsing — every Y level has the right block
// ---------------------------------------------------------------------------

func TestLayeredWorld_SubChunkParsing(t *testing.T) {
	sc := parseLayeredSubChunk(t)

	cases := []struct {
		y    int
		want string
	}{
		// bedrock
		{0, "minecraft:bedrock"},
		// deepslate
		{1, "minecraft:deepslate"},
		{2, "minecraft:deepslate"},
		{3, "minecraft:deepslate"},
		// stone
		{4, "minecraft:stone"},
		{5, "minecraft:stone"},
		{6, "minecraft:stone"},
		// dirt
		{7, "minecraft:dirt"},
		{8, "minecraft:dirt"},
		{9, "minecraft:dirt"},
		// grass (surface)
		{10, "minecraft:grass_block"},
		// air above surface
		{11, "minecraft:air"},
		{15, "minecraft:air"},
	}

	for _, tc := range cases {
		// sample one corner column per layer
		got := sc.Block(0, tc.y, 0).Name
		if got != tc.want {
			t.Errorf("Block(0,y=%d,0): got %q, want %q", tc.y, got, tc.want)
		}
	}

	// Also verify every column at each non-air Y has the right block
	// (confirms the 16×16 fill, not just column (0,0))
	for y := 0; y <= 10; y++ {
		want := expectedBlockAt(y)
		for x := 0; x < 16; x++ {
			for z := 0; z < 16; z++ {
				if got := sc.Block(x, y, z).Name; got != want {
					t.Errorf("Block(%d,y=%d,%d): got %q, want %q", x, y, z, got, want)
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Test 2: surface detection — grass at Y=10, 256 columns
// ---------------------------------------------------------------------------

func TestLayeredWorld_SurfaceDetection(t *testing.T) {
	sc := parseLayeredSubChunk(t)
	cd := layeredChunkData(sc)

	blocks := world.FindSurfaceBlocks(cd)

	if len(blocks) != 256 {
		t.Fatalf("want 256 surface blocks (16×16 columns), got %d", len(blocks))
	}
	for _, b := range blocks {
		if b.Y != 10 {
			t.Errorf("column (%d,%d): surface Y=%d, want 10 (grass layer)", b.X, b.Z, b.Y)
		}
		if b.Block.Name != "minecraft:grass_block" {
			t.Errorf("column (%d,%d): block=%q, want minecraft:grass_block", b.X, b.Z, b.Block.Name)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 3: full SQLite export pipeline
// ---------------------------------------------------------------------------

func TestLayeredWorld_SQLiteExport(t *testing.T) {
	sc := parseLayeredSubChunk(t)
	cd := layeredChunkData(sc)

	dbPath := t.TempDir() + "/world.db"
	db, err := export.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	ce, err := export.NewChunkExporter(db, true /*surfaceOnly*/)
	if err != nil {
		t.Fatalf("NewChunkExporter: %v", err)
	}
	if err := ce.ExportChunk(cd); err != nil {
		t.Fatalf("ExportChunk: %v", err)
	}
	if err := ce.Close(); err != nil {
		t.Fatalf("ChunkExporter.Close: %v", err)
	}

	// ---- Query the database ----
	rows, err := db.Query(
		`SELECT x, z, y, block_name FROM surface_blocks WHERE dimension=0 ORDER BY x, z`,
	)
	if err != nil {
		t.Fatalf("query surface_blocks: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var x, z, y int
		var name string
		if err := rows.Scan(&x, &z, &y, &name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		count++
		if y != 10 {
			t.Errorf("column (%d,%d) in DB: y=%d, want 10 (grass surface)", x, z, y)
		}
		if name != "minecraft:grass_block" {
			t.Errorf("column (%d,%d) in DB: block=%q, want minecraft:grass_block", x, z, name)
		}
		// x must be in [0,15], z in [0,15] (chunk 0,0)
		if x < 0 || x > 15 || z < 0 || z > 15 {
			t.Errorf("block (%d,%d): coordinates outside chunk (0,0) range [0,15]", x, z)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	if count != 256 {
		t.Errorf("want 256 rows in surface_blocks (16×16 columns), got %d", count)
	}
}
