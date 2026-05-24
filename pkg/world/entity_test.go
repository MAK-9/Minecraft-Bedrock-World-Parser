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

// buildItemPayload returns the raw compound map payload for one inventory item.
// It strips the 3-byte header (0x0A tag + 2-byte empty name) that buildNBTCompound
// prepends, leaving only the fields + TAG_End — the form required inside a TAG_List.
func buildItemPayload(id string, slot, count int8, damage int16) []byte {
	full := buildNBTCompound(map[string]nbtField{
		"id":     nbtStringField(id),
		"Slot":   nbtByteField(slot),
		"Count":  nbtByteField(count),
		"Damage": nbtInt16Field(damage),
	})
	return full[3:] // strip 0x0A + uint16(0) name length
}

func TestParseBlockEntities_ChestWithItems(t *testing.T) {
	items := [][]byte{
		buildItemPayload("minecraft:diamond_sword", 0, 1, 5),
		buildItemPayload("minecraft:torch", 13, 32, 0),
	}
	chestNBT := buildNBTCompound(map[string]nbtField{
		"id":    nbtStringField("Chest"),
		"x":     nbtInt32Field(8),
		"y":     nbtInt32Field(64),
		"z":     nbtInt32Field(-16),
		"Items": nbtListField(0x0A, items),
	})

	bes, err := ParseBlockEntities(chestNBT)
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
	if be.X != 8 || be.Y != 64 || be.Z != -16 {
		t.Errorf("position: got (%d,%d,%d), want (8,64,-16)", be.X, be.Y, be.Z)
	}

	raw, ok := be.RawData["Items"].([]interface{})
	if !ok {
		t.Fatalf("Items: expected []interface{}, got %T", be.RawData["Items"])
	}
	if len(raw) != 2 {
		t.Fatalf("Items: expected 2 entries, got %d", len(raw))
	}

	assertItem := func(idx int, wantID string, wantSlot, wantCount int8) {
		t.Helper()
		m, ok := raw[idx].(map[string]interface{})
		if !ok {
			t.Errorf("item %d: expected map, got %T", idx, raw[idx])
			return
		}
		if got, _ := m["id"].(string); got != wantID {
			t.Errorf("item %d id: got %q, want %q", idx, got, wantID)
		}
		if got, _ := m["Slot"].(int8); got != wantSlot {
			t.Errorf("item %d Slot: got %d, want %d", idx, got, wantSlot)
		}
		if got, _ := m["Count"].(int8); got != wantCount {
			t.Errorf("item %d Count: got %d, want %d", idx, got, wantCount)
		}
	}
	assertItem(0, "minecraft:diamond_sword", 0, 1)
	assertItem(1, "minecraft:torch", 13, 32)

	json := be.RawJSON()
	if len(json) == 0 {
		t.Error("RawJSON: got empty string")
	}
	if !bytes.Contains([]byte(json), []byte("Items")) {
		t.Errorf("RawJSON: missing Items key, got: %s", json)
	}
}
