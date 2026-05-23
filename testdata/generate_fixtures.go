//go:build ignore

// Run with: go run testdata/generate_fixtures.go
// Generates a minimal Bedrock LevelDB world in testdata/fixtures/minimal_world/
// used by integration tests.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"

	"github.com/df-mc/goleveldb/leveldb"
	"github.com/df-mc/goleveldb/leveldb/opt"
)

func main() {
	dst := "testdata/fixtures/minimal_world/db"
	if err := os.RemoveAll(dst); err != nil {
		panic(err)
	}

	db, err := leveldb.OpenFile(dst, &opt.Options{})
	if err != nil {
		panic(fmt.Sprintf("open: %v", err))
	}
	defer db.Close()

	// Write two chunks:
	//   chunk (0,0) Overworld — 1 subchunk (y=4), all stone
	//   chunk (1,0) Overworld — 1 subchunk (y=4), grass surface + stone below
	if err := writeChunk(db, 0, 0, 0, allStoneSubChunk(4)); err != nil {
		panic(err)
	}
	if err := writeChunk(db, 1, 0, 0, grassSurfaceSubChunk(4)); err != nil {
		panic(err)
	}

	// Write level.dat
	if err := writeLevelDat(); err != nil {
		panic(err)
	}

	fmt.Println("Fixtures written to testdata/fixtures/minimal_world/")
}

// writeChunk writes a single SubChunk key into the database.
func writeChunk(db *leveldb.DB, cx, cz, dim int32, subChunkData []byte) error {
	key := makeSubChunkKey(cx, cz, dim, 4)
	return db.Put(key, subChunkData, nil)
}

func makeSubChunkKey(cx, cz, dim int32, subY int8) []byte {
	var buf bytes.Buffer
	writeInt32LE(&buf, cx)
	writeInt32LE(&buf, cz)
	if dim != 0 {
		writeInt32LE(&buf, dim)
	}
	buf.WriteByte(0x2F) // TagSubChunk
	buf.WriteByte(byte(subY))
	return buf.Bytes()
}

func writeInt32LE(buf *bytes.Buffer, v int32) {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(v))
	buf.Write(b)
}

// allStoneSubChunk returns SubChunk v8 data with all 4096 blocks = minecraft:stone.
func allStoneSubChunk(subY int8) []byte {
	return buildSubChunkV8([]blockEntry{{name: "minecraft:stone"}}, allZeroIndices())
}

// grassSurfaceSubChunk returns SubChunk v8 data with y=15 (top) = grass, rest = stone.
func grassSurfaceSubChunk(subY int8) []byte {
	palette := []blockEntry{
		{name: "minecraft:stone"},
		{name: "minecraft:grass"},
	}
	var indices [4096]int
	for z := 0; z < 16; z++ {
		for x := 0; x < 16; x++ {
			indices[x+z*16+15*256] = 1 // y=15 → grass
		}
	}
	return buildSubChunkV8(palette, indices)
}

func allZeroIndices() [4096]int { return [4096]int{} }

type blockEntry struct{ name string }

func buildSubChunkV8(palette []blockEntry, indices [4096]int) []byte {
	var buf bytes.Buffer

	bitsPerBlock := 1
	for (1 << bitsPerBlock) < len(palette) {
		bitsPerBlock++
	}

	buf.WriteByte(8)                       // version
	buf.WriteByte(1)                       // layer count
	buf.WriteByte(byte((bitsPerBlock << 1) | 1)) // storage header

	blocksPerWord := 32 / bitsPerBlock
	wordCount := (4096 + blocksPerWord - 1) / blocksPerWord
	mask := uint32((1 << bitsPerBlock) - 1)
	words := make([]uint32, wordCount)
	for i, idx := range indices {
		wi := i / blocksPerWord
		bp := (i % blocksPerWord) * bitsPerBlock
		words[wi] |= (uint32(idx) & mask) << bp
	}
	tmp := make([]byte, 4)
	for _, w := range words {
		binary.LittleEndian.PutUint32(tmp, w)
		buf.Write(tmp)
	}

	binary.LittleEndian.PutUint32(tmp, uint32(len(palette)))
	buf.Write(tmp)
	for _, p := range palette {
		buf.Write(encodeBlockNBT(p.name))
	}
	return buf.Bytes()
}

func encodeBlockNBT(name string) []byte {
	var buf bytes.Buffer
	buf.WriteByte(0x0A)  // TAG_Compound
	writeStr(&buf, "")   // empty compound name
	// TAG_String "name"
	buf.WriteByte(0x08)
	writeStr(&buf, "name")
	writeStr(&buf, name)
	// TAG_Compound "states" (empty)
	buf.WriteByte(0x0A)
	writeStr(&buf, "states")
	buf.WriteByte(0x00)
	// TAG_End
	buf.WriteByte(0x00)
	return buf.Bytes()
}

func writeStr(buf *bytes.Buffer, s string) {
	tmp := make([]byte, 2)
	binary.LittleEndian.PutUint16(tmp, uint16(len(s)))
	buf.Write(tmp)
	buf.WriteString(s)
}

func writeLevelDat() error {
	nbt := buildLevelDatNBT("MinimalWorld", 0, 64, 0)
	var out bytes.Buffer
	tmp := make([]byte, 4)
	binary.LittleEndian.PutUint32(tmp, 10) // storage version
	out.Write(tmp)
	binary.LittleEndian.PutUint32(tmp, uint32(len(nbt)))
	out.Write(tmp)
	out.Write(nbt)

	dir := "testdata/fixtures/minimal_world"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(dir+"/level.dat", out.Bytes(), 0644)
}

func buildLevelDatNBT(name string, spawnX, spawnY, spawnZ int32) []byte {
	var buf bytes.Buffer
	buf.WriteByte(0x0A) // TAG_Compound
	writeStr(&buf, "")  // empty name

	fields := map[string]interface{}{
		"LevelName":      name,
		"StorageVersion": int32(10),
		"SpawnX":         spawnX,
		"SpawnY":         spawnY,
		"SpawnZ":         spawnZ,
		"GameType":       int32(0),
		"RandomSeed":     int64(42),
		"Difficulty":     int32(2),
	}
	for k, v := range fields {
		switch val := v.(type) {
		case string:
			buf.WriteByte(0x08)
			writeStr(&buf, k)
			writeStr(&buf, val)
		case int32:
			buf.WriteByte(0x03)
			writeStr(&buf, k)
			tmp := make([]byte, 4)
			binary.LittleEndian.PutUint32(tmp, uint32(val))
			buf.Write(tmp)
		case int64:
			buf.WriteByte(0x04)
			writeStr(&buf, k)
			tmp := make([]byte, 8)
			binary.LittleEndian.PutUint64(tmp, uint64(val))
			buf.Write(tmp)
		}
	}
	buf.WriteByte(0x00) // TAG_End
	return buf.Bytes()
}
