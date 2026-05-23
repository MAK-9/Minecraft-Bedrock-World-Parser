package world

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/df-mc/goleveldb/leveldb"
	"github.com/df-mc/goleveldb/leveldb/opt"
)

// writeTestWorld builds a tiny LevelDB world on disk with multiple chunks and
// returns its path. Used to exercise the streaming IterateChunks logic.
func writeTestWorld(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbDir := filepath.Join(dir, "db")

	db, err := leveldb.OpenFile(dbDir, &opt.Options{})
	if err != nil {
		t.Fatalf("opening test leveldb: %v", err)
	}

	// Three overworld chunks, each with one all-stone subchunk at subY=0.
	stone := buildSubChunkForTest()
	for _, c := range []struct{ x, z int32 }{{0, 0}, {1, 0}, {-1, 5}} {
		key := subChunkKeyForTest(c.x, c.z, 0)
		if err := db.Put(key, stone, nil); err != nil {
			t.Fatalf("put: %v", err)
		}
	}
	db.Close()

	// Minimal level.dat so world.Open works.
	writeMinimalLevelDat(t, dir)
	return dir
}

func subChunkKeyForTest(x, z int32, subY int8) []byte {
	var buf bytes.Buffer
	tmp := make([]byte, 4)
	binary.LittleEndian.PutUint32(tmp, uint32(x))
	buf.Write(tmp)
	binary.LittleEndian.PutUint32(tmp, uint32(z))
	buf.Write(tmp)
	buf.WriteByte(0x2F) // TagSubChunk
	buf.WriteByte(byte(subY))
	return buf.Bytes()
}

func buildSubChunkForTest() []byte {
	palette := []BlockState{{Name: "minecraft:stone"}}
	var indices [4096]int
	return buildSubChunkV8(palette, indices)
}

func writeMinimalLevelDat(t *testing.T, dir string) {
	t.Helper()
	nbt := buildNBTCompound(map[string]nbtField{
		"LevelName":      nbtStringField("StreamTest"),
		"StorageVersion": nbtInt32Field(10),
	})
	var buf bytes.Buffer
	tmp := make([]byte, 4)
	binary.LittleEndian.PutUint32(tmp, 10)
	buf.Write(tmp)
	binary.LittleEndian.PutUint32(tmp, uint32(len(nbt)))
	buf.Write(tmp)
	buf.Write(nbt)
	if err := os.WriteFile(filepath.Join(dir, "level.dat"), buf.Bytes(), 0644); err != nil {
		t.Fatalf("writing level.dat: %v", err)
	}
}

// TestIterateChunks_Streaming verifies that every chunk is visited exactly once
// and that each chunk's data is complete when streamed.
func TestIterateChunks_Streaming(t *testing.T) {
	worldPath := writeTestWorld(t)

	wr, err := Open(worldPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer wr.Close()

	seen := map[[2]int32]int{}
	err = wr.IterateChunks(-1, func(cd ChunkData) error {
		key := [2]int32{cd.Key.X, cd.Key.Z}
		seen[key]++
		if len(cd.SubChunks) != 1 {
			t.Errorf("chunk %v: expected 1 subchunk, got %d", key, len(cd.SubChunks))
		}
		// Verify decoded content is stone.
		if sc, ok := cd.SubChunks[0]; ok {
			if b := sc.Block(0, 0, 0); b.Name != "minecraft:stone" {
				t.Errorf("chunk %v block: got %q, want minecraft:stone", key, b.Name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("IterateChunks: %v", err)
	}

	if len(seen) != 3 {
		t.Errorf("expected 3 distinct chunks, got %d: %v", len(seen), seen)
	}
	// Each chunk must be visited exactly once (streaming must not split a chunk).
	for k, n := range seen {
		if n != 1 {
			t.Errorf("chunk %v visited %d times, want exactly 1", k, n)
		}
	}
}

// TestIterateChunks_DimensionFilter verifies the dimension filter.
func TestIterateChunks_DimensionFilter(t *testing.T) {
	worldPath := writeTestWorld(t)
	wr, err := Open(worldPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer wr.Close()

	count := 0
	err = wr.IterateChunks(0, func(cd ChunkData) error {
		if cd.Key.Dimension != DimOverworld {
			t.Errorf("got dimension %d, want Overworld", cd.Key.Dimension)
		}
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("IterateChunks: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 overworld chunks, got %d", count)
	}
}
