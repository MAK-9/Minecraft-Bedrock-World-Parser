package world

import (
	"bytes"
	"testing"
)

// buildBlockEntityNBT creates a minimal block entity NBT compound.
func buildBlockEntityNBT(id string, x, y, z int32) []byte {
	return buildNBTCompound(map[string]nbtField{
		"id": nbtStringField(id),
		"x":  nbtInt32Field(x),
		"y":  nbtInt32Field(y),
		"z":  nbtInt32Field(z),
	})
}

func TestParseBlockEntities_Single(t *testing.T) {
	data := buildBlockEntityNBT("Chest", 10, 64, -5)
	bes, err := ParseBlockEntities(data)
	if err != nil {
		t.Fatalf("ParseBlockEntities: %v", err)
	}
	if len(bes) != 1 {
		t.Fatalf("expected 1 block entity, got %d", len(bes))
	}
	be := bes[0]
	if be.Type != "Chest" {
		t.Errorf("Type: got %q, want Chest", be.Type)
	}
	if be.X != 10 || be.Y != 64 || be.Z != -5 {
		t.Errorf("position: got (%d,%d,%d), want (10,64,-5)", be.X, be.Y, be.Z)
	}
}

func TestParseBlockEntities_Multiple(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(buildBlockEntityNBT("Chest", 0, 64, 0))
	buf.Write(buildBlockEntityNBT("Sign", 5, 70, 5))
	buf.Write(buildBlockEntityNBT("Furnace", -3, 60, 2))

	bes, err := ParseBlockEntities(buf.Bytes())
	if err != nil {
		t.Fatalf("ParseBlockEntities: %v", err)
	}
	if len(bes) != 3 {
		t.Fatalf("expected 3 block entities, got %d", len(bes))
	}
	if bes[1].Type != "Sign" {
		t.Errorf("2nd entity: got %q, want Sign", bes[1].Type)
	}
}

func TestParseBlockEntities_Empty(t *testing.T) {
	bes, err := ParseBlockEntities([]byte{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bes) != 0 {
		t.Errorf("expected 0 entities, got %d", len(bes))
	}
}

func TestBlockEntity_RawJSON(t *testing.T) {
	data := buildBlockEntityNBT("Chest", 1, 2, 3)
	bes, err := ParseBlockEntities(data)
	if err != nil {
		t.Fatalf("ParseBlockEntities: %v", err)
	}
	json := bes[0].RawJSON()
	if len(json) == 0 || json == "{}" {
		t.Error("expected non-empty JSON with actual fields")
	}
}

func TestParseBlockEntities_PositiveZ(t *testing.T) {
	data := buildBlockEntityNBT("Sign", 0, 0, 100)
	bes, _ := ParseBlockEntities(data)
	if bes[0].Z != 100 {
		t.Errorf("Z: got %d, want 100", bes[0].Z)
	}
}
