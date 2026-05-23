package export

import (
	"database/sql"
	"fmt"

	"github.com/mak-9/minecraft-bedrock-world-parser/pkg/world"
	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS world_info (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS chunks (
    dimension INTEGER NOT NULL,
    chunk_x   INTEGER NOT NULL,
    chunk_z   INTEGER NOT NULL,
    PRIMARY KEY (dimension, chunk_x, chunk_z)
);

CREATE TABLE IF NOT EXISTS surface_blocks (
    dimension    INTEGER NOT NULL,
    chunk_x      INTEGER NOT NULL,
    chunk_z      INTEGER NOT NULL,
    x            INTEGER NOT NULL,
    z            INTEGER NOT NULL,
    y            INTEGER NOT NULL,
    block_name   TEXT    NOT NULL,
    block_states TEXT,
    PRIMARY KEY (dimension, x, z)
);

CREATE INDEX IF NOT EXISTS idx_surface_chunk
    ON surface_blocks(dimension, chunk_x, chunk_z);

CREATE TABLE IF NOT EXISTS biomes (
    dimension INTEGER NOT NULL,
    chunk_x   INTEGER NOT NULL,
    chunk_z   INTEGER NOT NULL,
    data      BLOB NOT NULL,
    PRIMARY KEY (dimension, chunk_x, chunk_z)
);

CREATE TABLE IF NOT EXISTS block_entities (
    dimension INTEGER NOT NULL,
    x         INTEGER NOT NULL,
    y         INTEGER NOT NULL,
    z         INTEGER NOT NULL,
    type      TEXT NOT NULL,
    data      TEXT NOT NULL,
    PRIMARY KEY (dimension, x, y, z)
);

CREATE TABLE IF NOT EXISTS entities (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    dimension INTEGER NOT NULL,
    chunk_x   INTEGER NOT NULL,
    chunk_z   INTEGER NOT NULL,
    x         REAL NOT NULL,
    y         REAL NOT NULL,
    z         REAL NOT NULL,
    type      TEXT NOT NULL,
    data      TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_entities_chunk
    ON entities(dimension, chunk_x, chunk_z);
`

// Options controls what is exported.
type Options struct {
	SurfaceOnly bool   // only export surface blocks (skip deep scanning)
	Dimension   int    // -1 = all, 0/1/2 = specific dimension
	Verbose     bool
}

// OpenDB opens (or creates) a SQLite database at path and applies the schema.
func OpenDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening SQLite: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("applying schema: %w", err)
	}
	// Performance pragmas
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("setting pragmas: %w", err)
	}
	return db, nil
}

// WriteWorldInfo inserts metadata rows.
func WriteWorldInfo(db *sql.DB, info *world.WorldInfo) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO world_info(key, value) VALUES(?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	rows := map[string]string{
		"level_name":      info.LevelName,
		"storage_version": fmt.Sprintf("%d", info.StorageVersion),
		"spawn_x":         fmt.Sprintf("%d", info.SpawnX),
		"spawn_y":         fmt.Sprintf("%d", info.SpawnY),
		"spawn_z":         fmt.Sprintf("%d", info.SpawnZ),
		"game_type":       fmt.Sprintf("%d", info.GameType),
		"seed":            fmt.Sprintf("%d", info.Seed),
		"difficulty":      fmt.Sprintf("%d", info.Difficulty),
	}
	for k, v := range rows {
		if _, err := stmt.Exec(k, v); err != nil {
			return fmt.Errorf("inserting world_info key %q: %w", k, err)
		}
	}
	return tx.Commit()
}

// ChunkExporter exports chunk data to SQLite using a long-lived transaction for performance.
type ChunkExporter struct {
	db          *sql.DB
	tx          *sql.Tx
	stmts       *exportStmts
	count       int
	batchSz     int
	surfaceOnly bool
}

type exportStmts struct {
	chunk       *sql.Stmt
	surface     *sql.Stmt
	biome       *sql.Stmt
	blockEntity *sql.Stmt
	entity      *sql.Stmt
}

// NewChunkExporter creates a ChunkExporter that batches inserts for performance.
// When surfaceOnly is true, only surface_blocks and biomes are written; entities
// and block entities are skipped.
func NewChunkExporter(db *sql.DB, surfaceOnly bool) (*ChunkExporter, error) {
	ce := &ChunkExporter{db: db, batchSz: 500, surfaceOnly: surfaceOnly}
	if err := ce.beginBatch(); err != nil {
		return nil, err
	}
	return ce, nil
}

func (ce *ChunkExporter) beginBatch() error {
	tx, err := ce.db.Begin()
	if err != nil {
		return err
	}

	stmts := &exportStmts{}
	stmts.chunk, err = tx.Prepare(`INSERT OR IGNORE INTO chunks(dimension, chunk_x, chunk_z) VALUES(?,?,?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	stmts.surface, err = tx.Prepare(`INSERT OR REPLACE INTO surface_blocks(dimension, chunk_x, chunk_z, x, z, y, block_name, block_states) VALUES(?,?,?,?,?,?,?,?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	stmts.biome, err = tx.Prepare(`INSERT OR REPLACE INTO biomes(dimension, chunk_x, chunk_z, data) VALUES(?,?,?,?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	stmts.blockEntity, err = tx.Prepare(`INSERT OR REPLACE INTO block_entities(dimension, x, y, z, type, data) VALUES(?,?,?,?,?,?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	stmts.entity, err = tx.Prepare(`INSERT INTO entities(dimension, chunk_x, chunk_z, x, y, z, type, data) VALUES(?,?,?,?,?,?,?,?)`)
	if err != nil {
		tx.Rollback()
		return err
	}

	ce.tx = tx
	ce.stmts = stmts
	return nil
}

func (ce *ChunkExporter) commitBatch() error {
	ce.stmts.chunk.Close()
	ce.stmts.surface.Close()
	ce.stmts.biome.Close()
	ce.stmts.blockEntity.Close()
	ce.stmts.entity.Close()
	return ce.tx.Commit()
}

// ExportChunk writes all data for one chunk to the current batch transaction.
func (ce *ChunkExporter) ExportChunk(cd world.ChunkData) error {
	dim := int(cd.Key.Dimension)
	cx := int(cd.Key.X)
	cz := int(cd.Key.Z)

	if _, err := ce.stmts.chunk.Exec(dim, cx, cz); err != nil {
		return fmt.Errorf("inserting chunk (%d,%d): %w", cx, cz, err)
	}

	// Surface blocks
	for _, sb := range world.FindSurfaceBlocks(cd) {
		if _, err := ce.stmts.surface.Exec(dim, cx, cz, sb.X, sb.Z, sb.Y, sb.Block.Name, sb.Block.StatesJSON()); err != nil {
			return fmt.Errorf("inserting surface block: %w", err)
		}
	}

	// Biome data
	if cd.Data2D != nil {
		if _, err := ce.stmts.biome.Exec(dim, cx, cz, cd.Data2D.BiomeIDs[:]); err != nil {
			return fmt.Errorf("inserting biome: %w", err)
		}
	}

	if !ce.surfaceOnly {
		// Block entities
		for _, be := range cd.BlockEntities {
			if _, err := ce.stmts.blockEntity.Exec(dim, be.X, be.Y, be.Z, be.Type, be.RawJSON()); err != nil {
				return fmt.Errorf("inserting block entity: %w", err)
			}
		}

		// Entities
		for _, e := range cd.Entities {
			if _, err := ce.stmts.entity.Exec(dim, cx, cz, e.X, e.Y, e.Z, e.Type, e.RawJSON()); err != nil {
				return fmt.Errorf("inserting entity: %w", err)
			}
		}
	}

	ce.count++
	if ce.count%ce.batchSz == 0 {
		if err := ce.commitBatch(); err != nil {
			return err
		}
		if err := ce.beginBatch(); err != nil {
			return err
		}
	}
	return nil
}

// Close commits any remaining batch and closes statements.
func (ce *ChunkExporter) Close() error {
	return ce.commitBatch()
}
