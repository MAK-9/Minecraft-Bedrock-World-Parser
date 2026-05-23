package world

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// buildLevelDat creates a minimal level.dat binary.
func buildLevelDat(fields map[string]nbtField) []byte {
	nbt := buildNBTCompound(fields)

	var buf bytes.Buffer
	tmp := make([]byte, 4)

	binary.LittleEndian.PutUint32(tmp, 10) // storage version
	buf.Write(tmp)
	binary.LittleEndian.PutUint32(tmp, uint32(len(nbt)))
	buf.Write(tmp)
	buf.Write(nbt)
	return buf.Bytes()
}

func TestParseLevelDat_Basic(t *testing.T) {
	data := buildLevelDat(map[string]nbtField{
		"LevelName":      nbtStringField("TestWorld"),
		"StorageVersion": nbtInt32Field(10),
		"SpawnX":         nbtInt32Field(100),
		"SpawnY":         nbtInt32Field(64),
		"SpawnZ":         nbtInt32Field(-200),
		"GameType":       nbtInt32Field(1),
		"RandomSeed":     nbtInt64Field(123456789),
		"Difficulty":     nbtInt32Field(2),
	})

	info, err := ParseLevelDat(data)
	if err != nil {
		t.Fatalf("ParseLevelDat: %v", err)
	}

	if info.LevelName != "TestWorld" {
		t.Errorf("LevelName: got %q, want TestWorld", info.LevelName)
	}
	if info.SpawnX != 100 || info.SpawnY != 64 || info.SpawnZ != -200 {
		t.Errorf("Spawn: got (%d,%d,%d), want (100,64,-200)", info.SpawnX, info.SpawnY, info.SpawnZ)
	}
	if info.GameType != 1 {
		t.Errorf("GameType: got %d, want 1 (Creative)", info.GameType)
	}
	if info.Seed != 123456789 {
		t.Errorf("Seed: got %d, want 123456789", info.Seed)
	}
	if info.Difficulty != 2 {
		t.Errorf("Difficulty: got %d, want 2 (Normal)", info.Difficulty)
	}
}

func TestParseLevelDat_TooShort(t *testing.T) {
	if _, err := ParseLevelDat([]byte{1, 2, 3}); err == nil {
		t.Error("expected error for too-short data")
	}
}

func TestParseLevelDat_GameTypeName(t *testing.T) {
	cases := []struct {
		gt   int32
		name string
	}{
		{0, "Survival"},
		{1, "Creative"},
		{2, "Adventure"},
		{3, "Spectator"},
	}
	for _, c := range cases {
		info := &WorldInfo{GameType: c.gt}
		if got := info.GameTypeName(); got != c.name {
			t.Errorf("GameType %d: got %q, want %q", c.gt, got, c.name)
		}
	}
}

func TestParseLevelDat_MissingFields(t *testing.T) {
	// Minimal level.dat with only LevelName
	data := buildLevelDat(map[string]nbtField{
		"LevelName": nbtStringField("Minimal"),
	})
	info, err := ParseLevelDat(data)
	if err != nil {
		t.Fatalf("ParseLevelDat: %v", err)
	}
	if info.LevelName != "Minimal" {
		t.Errorf("LevelName: got %q, want Minimal", info.LevelName)
	}
	// Other fields should have zero values
	if info.SpawnX != 0 || info.SpawnY != 0 || info.SpawnZ != 0 {
		t.Errorf("expected zero spawn, got (%d,%d,%d)", info.SpawnX, info.SpawnY, info.SpawnZ)
	}
}
