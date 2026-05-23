package world

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/df-mc/goleveldb/leveldb"
	"github.com/df-mc/goleveldb/leveldb/opt"
)

// ChunkData holds all decoded data for one chunk coordinate.
type ChunkData struct {
	Key         ChunkKey
	SubChunks   map[int8]*SubChunk  // subchunk Y index → SubChunk
	Data2D      *Data2D             // height map + 2D biomes (may be nil for new worlds)
	BlockEntities []BlockEntity
	Entities    []Entity
}

// WorldReader opens and iterates a Minecraft Bedrock world stored in LevelDB.
type WorldReader struct {
	db      *leveldb.DB
	Info    *WorldInfo
	worldPath string
}

// Open opens the Bedrock world at the given path.
// The world directory must contain level.dat and a db/ sub-directory.
func Open(worldPath string) (*WorldReader, error) {
	info, err := ReadLevelDat(worldPath)
	if err != nil {
		return nil, err
	}

	dbPath := filepath.Join(worldPath, "db")
	db, err := leveldb.OpenFile(dbPath, &opt.Options{
		Compression: opt.FlateCompression,
	})
	if err != nil {
		return nil, fmt.Errorf("opening LevelDB at %s: %w", dbPath, err)
	}

	return &WorldReader{db: db, Info: info, worldPath: worldPath}, nil
}

// Close releases the LevelDB handle.
func (wr *WorldReader) Close() error {
	return wr.db.Close()
}

// IterateChunks calls fn for every chunk in the given dimension.
// Pass dim == -1 to iterate all dimensions.
// Errors from fn stop iteration and are returned.
//
// Chunks are processed in a streaming fashion: LevelDB returns keys in sorted
// order, so all keys belonging to one chunk coordinate (x, z, dimension) are
// contiguous. We accumulate a single chunk at a time and flush it to fn as soon
// as the coordinate changes, keeping memory usage bounded to one chunk rather
// than the whole world.
func (wr *WorldReader) IterateChunks(dim int, fn func(ChunkData) error) error {
	type chunkAccumulator struct {
		subChunks     map[int8]*SubChunk
		data2D        *Data2D
		blockEntities []BlockEntity
		entities      []Entity
	}

	type coord struct {
		X, Z      int32
		Dimension Dimension
	}

	iter := wr.db.NewIterator(nil, nil)
	defer iter.Release()

	var (
		curCoord coord
		curAcc   *chunkAccumulator
		haveCur  bool
	)

	flush := func() error {
		if !haveCur || curAcc == nil {
			return nil
		}
		cd := ChunkData{
			Key: ChunkKey{
				X:         curCoord.X,
				Z:         curCoord.Z,
				Dimension: curCoord.Dimension,
			},
			SubChunks:     curAcc.subChunks,
			Data2D:        curAcc.data2D,
			BlockEntities: curAcc.blockEntities,
			Entities:      curAcc.entities,
		}
		err := fn(cd)
		curAcc = nil // release for GC
		return err
	}

	for iter.Next() {
		rawKey := iter.Key()
		ck, ok := ParseChunkKey(rawKey)
		if !ok {
			continue
		}
		if dim != -1 && int(ck.Dimension) != dim {
			continue
		}

		c := coord{X: ck.X, Z: ck.Z, Dimension: ck.Dimension}
		if !haveCur || c != curCoord {
			if err := flush(); err != nil {
				return err
			}
			curCoord = c
			curAcc = &chunkAccumulator{subChunks: make(map[int8]*SubChunk)}
			haveCur = true
		}

		val := iter.Value()

		switch ck.Tag {
		case TagSubChunk:
			sc, err := ParseSubChunk(val)
			if err != nil {
				continue // non-fatal: skip undecodable subchunk
			}
			curAcc.subChunks[ck.SubY] = sc

		case TagData2D:
			d2d, err := ParseData2D(val)
			if err != nil {
				continue
			}
			curAcc.data2D = d2d

		case TagBlockEntity:
			bes, err := ParseBlockEntities(val)
			if err != nil {
				continue
			}
			curAcc.blockEntities = append(curAcc.blockEntities, bes...)

		case TagEntity:
			ents, err := ParseEntities(val)
			if err != nil {
				continue
			}
			curAcc.entities = append(curAcc.entities, ents...)
		}
	}

	if err := iter.Error(); err != nil {
		return fmt.Errorf("LevelDB iteration error: %w", err)
	}

	// Flush the final chunk.
	return flush()
}

// SurfaceBlock holds a single surface block result from FindSurfaceBlocks.
type SurfaceBlock struct {
	X, Z      int    // absolute world coordinates
	Y         int    // surface height (highest non-air block Y)
	Block     BlockState
}

// FindSurfaceBlocks returns the highest non-air block for each XZ column in a chunk.
// It uses the height map from Data2D when available; otherwise it scans SubChunks top-down.
func FindSurfaceBlocks(cd ChunkData) []SurfaceBlock {
	chunkBaseX := int(cd.Key.X) * 16
	chunkBaseZ := int(cd.Key.Z) * 16

	results := make([]SurfaceBlock, 0, 256)

	for lz := 0; lz < 16; lz++ {
		for lx := 0; lx < 16; lx++ {
			b, y := findSurfaceAt(cd, lx, lz)
			if IsAir(b) {
				continue
			}
			results = append(results, SurfaceBlock{
				X:     chunkBaseX + lx,
				Z:     chunkBaseZ + lz,
				Y:     y,
				Block: b,
			})
		}
	}
	return results
}

// findSurfaceAt finds the highest non-air block at local column (lx, lz).
func findSurfaceAt(cd ChunkData, lx, lz int) (BlockState, int) {
	// Collect and sort subchunk Y indices from highest to lowest.
	subYs := make([]int8, 0, len(cd.SubChunks))
	for sy := range cd.SubChunks {
		subYs = append(subYs, sy)
	}
	// Sort descending
	for i := 0; i < len(subYs)-1; i++ {
		for j := i + 1; j < len(subYs); j++ {
			if subYs[j] > subYs[i] {
				subYs[i], subYs[j] = subYs[j], subYs[i]
			}
		}
	}

	for _, sy := range subYs {
		sc := cd.SubChunks[sy]
		baseY := int(sy) * 16
		// Scan from top of subchunk down
		for ly := 15; ly >= 0; ly-- {
			b := sc.Block(lx, ly, lz)
			if !IsAir(b) {
				return b, baseY + ly
			}
		}
	}
	return AirBlock, 0
}

// CopyWorld copies the world directory to dstPath (for --copy-first mode).
// Returns the path to the copy.
func CopyWorld(srcPath, dstPath string) error {
	return copyDir(srcPath, dstPath)
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
