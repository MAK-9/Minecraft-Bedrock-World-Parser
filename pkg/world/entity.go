package world

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Entity represents a Minecraft entity (mob, item, …) decoded from LevelDB.
type Entity struct {
	Type string
	X, Y, Z float64
	RawData map[string]interface{} // full NBT data as a Go map
}

// BlockEntity represents a tile entity (chest, sign, furnace, …).
type BlockEntity struct {
	Type    string
	X, Y, Z int32
	RawData map[string]interface{}
}

// RawJSON returns the entity's full NBT data as a JSON string.
func (e Entity) RawJSON() string {
	data, _ := json.Marshal(e.RawData)
	return string(data)
}

// RawJSON returns the block entity's full NBT data as a JSON string.
func (be BlockEntity) RawJSON() string {
	data, _ := json.Marshal(be.RawData)
	return string(data)
}

// ParseBlockEntities decodes all block entities from a TagBlockEntity LevelDB value.
// The value is a sequence of little-endian NBT compound tags.
func ParseBlockEntities(data []byte) ([]BlockEntity, error) {
	r := bytes.NewReader(data)
	var result []BlockEntity

	for r.Len() > 0 {
		raw, err := readRawNBTCompound(r)
		if err != nil {
			return nil, fmt.Errorf("block entity %d: %w", len(result), err)
		}

		be := BlockEntity{RawData: raw}
		if id, ok := raw["id"].(string); ok {
			be.Type = id
		}
		if x, ok := toInt32(raw["x"]); ok {
			be.X = x
		}
		if y, ok := toInt32(raw["y"]); ok {
			be.Y = y
		}
		if z, ok := toInt32(raw["z"]); ok {
			be.Z = z
		}
		result = append(result, be)
	}
	return result, nil
}

// ParseEntities decodes all entities from a TagEntity LevelDB value.
func ParseEntities(data []byte) ([]Entity, error) {
	r := bytes.NewReader(data)
	var result []Entity

	for r.Len() > 0 {
		raw, err := readRawNBTCompound(r)
		if err != nil {
			return nil, fmt.Errorf("entity %d: %w", len(result), err)
		}

		e := Entity{RawData: raw}
		if id, ok := raw["identifier"].(string); ok {
			e.Type = id
		}
		if pos, ok := raw["Pos"].([]interface{}); ok && len(pos) == 3 {
			e.X, _ = toFloat64(pos[0])
			e.Y, _ = toFloat64(pos[1])
			e.Z, _ = toFloat64(pos[2])
		}
		result = append(result, e)
	}
	return result, nil
}

// readRawNBTCompound reads one Bedrock LE NBT compound from r into a generic map.
func readRawNBTCompound(r *bytes.Reader) (map[string]interface{}, error) {
	tagType, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	if tagType != 0x0A {
		return nil, fmt.Errorf("expected TAG_Compound (0x0A), got 0x%02X", tagType)
	}
	// skip compound name
	if _, err := readNBTString(r); err != nil {
		return nil, err
	}
	return readMapPayload(r)
}

// readMapPayload reads compound payload into a map until TAG_End.
func readMapPayload(r *bytes.Reader) (map[string]interface{}, error) {
	m := make(map[string]interface{})
	for {
		tagType, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		if tagType == 0x00 {
			return m, nil
		}
		name, err := readNBTString(r)
		if err != nil {
			return nil, err
		}
		val, err := readAnyNBTValue(r, tagType)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", name, err)
		}
		m[name] = val
	}
}

// readAnyNBTValue reads any NBT value by type, returning a Go type.
func readAnyNBTValue(r *bytes.Reader, tagType byte) (interface{}, error) {
	switch tagType {
	case 0x01:
		b, err := r.ReadByte()
		return int8(b), err
	case 0x02:
		return readInt16LE(r)
	case 0x03:
		return readInt32LE(r)
	case 0x04:
		return readInt64LE(r)
	case 0x05:
		return readFloat32LE(r)
	case 0x06:
		return readFloat64LE(r)
	case 0x07: // TAG_Byte_Array
		n, err := readInt32LE(r)
		if err != nil {
			return nil, err
		}
		if n < 0 || int(n) > r.Len() {
			return nil, fmt.Errorf("byte array length %d exceeds remaining %d bytes", n, r.Len())
		}
		buf := make([]byte, n)
		_, err = r.Read(buf)
		return buf, err
	case 0x08:
		return readNBTString(r)
	case 0x09: // TAG_List
		elemType, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		count, err := readInt32LE(r)
		if err != nil {
			return nil, err
		}
		if count < 0 || int(count) > r.Len() {
			return nil, fmt.Errorf("list count %d exceeds remaining %d bytes", count, r.Len())
		}
		list := make([]interface{}, count)
		for i := int32(0); i < count; i++ {
			v, err := readAnyNBTValue(r, elemType)
			if err != nil {
				return nil, err
			}
			list[i] = v
		}
		return list, nil
	case 0x0A: // TAG_Compound
		return readMapPayload(r)
	case 0x0B: // TAG_Int_Array
		n, err := readInt32LE(r)
		if err != nil {
			return nil, err
		}
		if n < 0 || int(n)*4 > r.Len() {
			return nil, fmt.Errorf("int array length %d exceeds remaining %d bytes", n, r.Len())
		}
		arr := make([]int32, n)
		for i := int32(0); i < n; i++ {
			v, err := readInt32LE(r)
			if err != nil {
				return nil, err
			}
			arr[i] = v
		}
		return arr, nil
	case 0x0C: // TAG_Long_Array
		n, err := readInt32LE(r)
		if err != nil {
			return nil, err
		}
		if n < 0 || int(n)*8 > r.Len() {
			return nil, fmt.Errorf("long array length %d exceeds remaining %d bytes", n, r.Len())
		}
		arr := make([]int64, n)
		for i := int32(0); i < n; i++ {
			v, err := readInt64LE(r)
			if err != nil {
				return nil, err
			}
			arr[i] = v
		}
		return arr, nil
	default:
		return nil, fmt.Errorf("unknown NBT tag type 0x%02X", tagType)
	}
}

// --- helpers ---

func toInt32(v interface{}) (int32, bool) {
	switch x := v.(type) {
	case int32:
		return x, true
	case int8:
		return int32(x), true
	case int16:
		return int32(x), true
	case int64:
		return int32(x), true
	}
	return 0, false
}

func toFloat64(v interface{}) (float64, bool) {
	switch x := v.(type) {
	case float32:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}
