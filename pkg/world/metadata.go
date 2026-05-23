package world

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
)

// WorldInfo contains key metadata read from level.dat.
type WorldInfo struct {
	LevelName      string
	StorageVersion int32
	SpawnX         int32
	SpawnY         int32
	SpawnZ         int32
	GameType       int32 // 0=survival, 1=creative, 2=adventure, 3=spectator
	Seed           int64
	Difficulty     int32 // 0=peaceful, 1=easy, 2=normal, 3=hard
}

// GameTypeName returns a human-readable game mode string.
func (wi WorldInfo) GameTypeName() string {
	switch wi.GameType {
	case 0:
		return "Survival"
	case 1:
		return "Creative"
	case 2:
		return "Adventure"
	case 3:
		return "Spectator"
	default:
		return fmt.Sprintf("Unknown(%d)", wi.GameType)
	}
}

// ParseLevelDat parses the raw bytes of a Bedrock level.dat file.
//
// Format: [4 bytes: storage version LE] [4 bytes: payload size LE] [NBT compound LE].
func ParseLevelDat(data []byte) (*WorldInfo, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("level.dat too short: %d bytes", len(data))
	}

	payloadSize := binary.LittleEndian.Uint32(data[4:8])
	if uint32(len(data)) < 8+payloadSize {
		return nil, fmt.Errorf("level.dat truncated: header says %d payload bytes, got %d", payloadSize, len(data)-8)
	}

	nbtData := data[8 : 8+payloadSize]
	r := bytes.NewReader(nbtData)

	raw, err := readRawNBTCompound(r)
	if err != nil {
		return nil, fmt.Errorf("decoding level.dat NBT: %w", err)
	}

	info := &WorldInfo{}

	if v, ok := raw["StorageVersion"].(int32); ok {
		info.StorageVersion = v
	}
	if v, ok := raw["LevelName"].(string); ok {
		info.LevelName = v
	}
	if v, ok := raw["SpawnX"].(int32); ok {
		info.SpawnX = v
	}
	if v, ok := raw["SpawnY"].(int32); ok {
		info.SpawnY = v
	}
	if v, ok := raw["SpawnZ"].(int32); ok {
		info.SpawnZ = v
	}
	if v, ok := raw["GameType"].(int32); ok {
		info.GameType = v
	}
	if v, ok := raw["RandomSeed"].(int64); ok {
		info.Seed = v
	}
	if v, ok := raw["Difficulty"].(int32); ok {
		info.Difficulty = v
	}

	return info, nil
}

// ReadLevelDat reads and parses level.dat from the given world directory.
func ReadLevelDat(worldPath string) (*WorldInfo, error) {
	path := filepath.Join(worldPath, "level.dat")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading level.dat: %w", err)
	}
	return ParseLevelDat(data)
}
