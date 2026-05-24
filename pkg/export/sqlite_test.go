package export_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/mak-9/minecraft-bedrock-world-parser/pkg/export"
	"github.com/mak-9/minecraft-bedrock-world-parser/pkg/world"
)

// minimalWorldPath resolves the path to our test fixture relative to the repo root.
func minimalWorldPath(t *testing.T) string {
	t.Helper()
	_, filename, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(filename), "..", "..")
	return filepath.Join(repoRoot, "testdata", "fixtures", "minimal_world")
}

func TestExport_Integration(t *testing.T) {
	fixturePath := minimalWorldPath(t)
	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		t.Skip("testdata/fixtures/minimal_world not found — run: go run testdata/generate_fixtures.go")
	}

	// Copy the fixture to a temp dir so the test never modifies the committed files.
	worldPath := filepath.Join(t.TempDir(), "minimal_world")
	if err := world.CopyWorld(fixturePath, worldPath); err != nil {
		t.Fatalf("copying fixture: %v", err)
	}

	wr, err := world.Open(worldPath)
	if err != nil {
		t.Fatalf("world.Open: %v", err)
	}
	defer wr.Close()

	// Write to an in-memory (temp file) SQLite database.
	tmpFile, err := os.CreateTemp(t.TempDir(), "*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	db, err := export.OpenDB(tmpFile.Name())
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	if err := export.WriteWorldInfo(db, wr.Info); err != nil {
		t.Fatalf("WriteWorldInfo: %v", err)
	}

	exporter, err := export.NewChunkExporter(db, false)
	if err != nil {
		t.Fatalf("NewChunkExporter: %v", err)
	}

	if err := wr.IterateChunks(-1, exporter.ExportChunk); err != nil {
		t.Fatalf("IterateChunks: %v", err)
	}
	if err := exporter.Close(); err != nil {
		t.Fatalf("exporter.Close: %v", err)
	}

	// --- Assertions ---
	assertWorldInfo(t, db, "level_name", "MinimalWorld")
	assertWorldInfo(t, db, "seed", "42")

	// We have 2 chunks in the fixture.
	var chunkCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM chunks`).Scan(&chunkCount); err != nil {
		t.Fatalf("querying chunks: %v", err)
	}
	if chunkCount != 2 {
		t.Errorf("chunks: got %d, want 2", chunkCount)
	}

	// Each chunk contributes 256 surface blocks (16×16 columns).
	var surfaceCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM surface_blocks`).Scan(&surfaceCount); err != nil {
		t.Fatalf("querying surface_blocks: %v", err)
	}
	if surfaceCount != 512 {
		t.Errorf("surface_blocks: got %d, want 512 (2 chunks × 256 columns)", surfaceCount)
	}

	// Chunk (0,0): all blocks are stone — surface should be minecraft:stone at y=68 (subY=4, ly=15)
	var blockName string
	if err := db.QueryRow(`SELECT block_name FROM surface_blocks WHERE dimension=0 AND x=0 AND z=0`).Scan(&blockName); err != nil {
		t.Fatalf("querying surface block (0,0): %v", err)
	}
	if blockName != "minecraft:stone" {
		t.Errorf("surface block (0,0): got %q, want minecraft:stone", blockName)
	}

	// Chunk (1,0): top layer (y=15 of subY=4 = absolute y=79) is grass.
	if err := db.QueryRow(`SELECT block_name FROM surface_blocks WHERE dimension=0 AND x=16 AND z=0`).Scan(&blockName); err != nil {
		t.Fatalf("querying surface block (16,0): %v", err)
	}
	if blockName != "minecraft:grass" {
		t.Errorf("surface block (16,0): got %q, want minecraft:grass", blockName)
	}
}

func assertWorldInfo(t *testing.T, db *sql.DB, key, want string) {
	t.Helper()
	var got string
	if err := db.QueryRow(`SELECT value FROM world_info WHERE key=?`, key).Scan(&got); err != nil {
		t.Fatalf("world_info[%q]: %v", key, err)
	}
	if got != want {
		t.Errorf("world_info[%q]: got %q, want %q", key, got, want)
	}
}
