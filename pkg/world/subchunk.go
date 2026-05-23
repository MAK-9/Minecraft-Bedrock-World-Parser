package world

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
)

// BlockState represents a Minecraft block with its name and NBT state properties.
type BlockState struct {
	Name   string
	States map[string]interface{}
}

// AirBlock is the zero-value block used for air and empty positions.
var AirBlock = BlockState{Name: "minecraft:air"}

// IsAir reports whether b is an air-like block.
func IsAir(b BlockState) bool {
	return b.Name == "minecraft:air" || b.Name == "air" || b.Name == ""
}

// StatesJSON returns the block states encoded as a compact JSON string.
// Returns "" when there are no states.
func (bs BlockState) StatesJSON() string {
	if len(bs.States) == 0 {
		return ""
	}
	data, _ := json.Marshal(bs.States)
	return string(data)
}

// SubChunk holds the decoded 16×16×16 block data for one vertical section.
// Blocks are indexed by linearIndex(x, z, y) = x + z*16 + y*256.
type SubChunk struct {
	blocks [4096]BlockState
}

// Block returns the block at local coordinates (x, y, z), each in [0, 16).
func (sc *SubChunk) Block(x, y, z int) BlockState {
	return sc.blocks[x+z*16+y*256]
}

// Blocks returns the raw block array; index as x + z*16 + y*256.
func (sc *SubChunk) Blocks() *[4096]BlockState {
	return &sc.blocks
}

// ParseSubChunk decodes a raw SubChunk value read from LevelDB.
// Supports storage versions 8 and 9 (current Bedrock format).
func ParseSubChunk(data []byte) (*SubChunk, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("subchunk data too short (%d bytes)", len(data))
	}

	version := data[0]
	layerCount := int(data[1])

	switch version {
	case 8, 9:
		// supported
	default:
		return nil, fmt.Errorf("unsupported subchunk version %d (only v8/v9 supported)", version)
	}

	// v9 stores the subchunk Y index as byte 2 — we skip it (caller already knows it from the key).
	offset := 2
	if version == 9 {
		offset = 3
	}

	if layerCount == 0 {
		sc := &SubChunk{}
		for i := range sc.blocks {
			sc.blocks[i] = AirBlock
		}
		return sc, nil
	}

	// Decode the first (solid) storage layer. Additional layers (water, …) are ignored for 2D maps.
	layer, _, err := parseBlockStorage(data[offset:])
	if err != nil {
		return nil, fmt.Errorf("parsing block storage layer 0: %w", err)
	}

	sc := &SubChunk{}
	for i := 0; i < 4096; i++ {
		idx := layer.indices[i]
		if idx < len(layer.palette) {
			sc.blocks[i] = layer.palette[idx]
		} else {
			sc.blocks[i] = AirBlock
		}
	}
	return sc, nil
}

// blockStorage is an intermediate result from parseBlockStorage.
type blockStorage struct {
	palette []BlockState
	indices []int // 4096 palette indices, one per block position
}

func parseBlockStorage(data []byte) (blockStorage, int, error) {
	if len(data) < 1 {
		return blockStorage{}, 0, fmt.Errorf("empty block storage data")
	}

	raw := data[0]
	bitsPerBlock := int(raw >> 1)
	// isPersistent := raw & 1  (disk format always sets this bit)

	var wordCount int
	var blocksPerWord int

	if bitsPerBlock == 0 {
		// All blocks are the same palette entry — no word data.
		wordCount = 0
		blocksPerWord = 4096
	} else {
		blocksPerWord = 32 / bitsPerBlock
		wordCount = (4096 + blocksPerWord - 1) / blocksPerWord
	}

	headerSize := 1
	wordBytes := wordCount * 4

	if len(data) < headerSize+wordBytes+4 {
		return blockStorage{}, 0, fmt.Errorf(
			"block storage data too short: need %d bytes for header+words+palette_size, got %d",
			headerSize+wordBytes+4, len(data))
	}

	// Unpack block indices from packed uint32 words.
	indices := make([]int, 4096)
	if bitsPerBlock > 0 {
		mask := uint32((1 << bitsPerBlock) - 1)
		blockIdx := 0
		for w := 0; w < wordCount && blockIdx < 4096; w++ {
			word := binary.LittleEndian.Uint32(data[headerSize+w*4:])
			for b := 0; b < blocksPerWord && blockIdx < 4096; b++ {
				indices[blockIdx] = int((word >> (b * bitsPerBlock)) & mask)
				blockIdx++
			}
		}
	}
	// bitsPerBlock == 0: all indices stay 0 (palette[0] for everyone)

	// Read palette.
	paletteOffset := headerSize + wordBytes
	paletteSize := int(binary.LittleEndian.Uint32(data[paletteOffset:]))
	paletteOffset += 4

	if paletteSize < 1 || paletteSize > 4096 {
		return blockStorage{}, 0, fmt.Errorf("invalid palette size: %d", paletteSize)
	}

	palette, paletteBytes, err := decodePalette(data[paletteOffset:], paletteSize)
	if err != nil {
		return blockStorage{}, 0, fmt.Errorf("decoding palette: %w", err)
	}

	totalRead := paletteOffset + paletteBytes
	return blockStorage{palette: palette, indices: indices}, totalRead, nil
}

// nbtCompound is a minimal representation of a Bedrock block state NBT compound.
type nbtCompound struct {
	Name    string                 `nbt:"name"`
	States  map[string]interface{} `nbt:"states"`
	Version int32                  `nbt:"version,omitempty"`
}

// decodePalette reads count NBT compound tags sequentially from data.
// It returns the decoded palette and the number of bytes consumed.
func decodePalette(data []byte, count int) ([]BlockState, int, error) {
	r := bytes.NewReader(data)
	palette := make([]BlockState, 0, count)

	for i := 0; i < count; i++ {
		entry, err := decodeNBTCompound(r)
		if err != nil {
			return nil, 0, fmt.Errorf("palette entry %d: %w", i, err)
		}
		palette = append(palette, BlockState{
			Name:   entry.Name,
			States: entry.States,
		})
	}

	bytesRead := int(int64(len(data)) - int64(r.Len()))
	return palette, bytesRead, nil
}

// decodeNBTCompound reads one Bedrock little-endian NBT compound from r.
// The compound is decoded manually to avoid pulling in the full gophertunnel NBT package
// in this critical hot path. Only the fields we need (name, states) are extracted.
func decodeNBTCompound(r *bytes.Reader) (nbtCompound, error) {
	var out nbtCompound
	out.States = make(map[string]interface{})

	// Expect TAG_Compound (0x0A)
	tagType, err := r.ReadByte()
	if err != nil {
		return out, fmt.Errorf("reading tag type: %w", err)
	}
	if tagType != 0x0A {
		return out, fmt.Errorf("expected TAG_Compound (0x0A), got 0x%02X", tagType)
	}

	// Read the compound's own name (usually empty for palette entries)
	if _, err := readNBTString(r); err != nil {
		return out, fmt.Errorf("reading compound name: %w", err)
	}

	// Read compound payload until TAG_End
	if err := readCompoundPayload(r, &out); err != nil {
		return out, err
	}
	return out, nil
}

// readCompoundPayload reads tag-value pairs until TAG_End.
func readCompoundPayload(r *bytes.Reader, out *nbtCompound) error {
	for {
		tagType, err := r.ReadByte()
		if err != nil {
			return fmt.Errorf("reading tag type in compound: %w", err)
		}
		if tagType == 0x00 { // TAG_End
			return nil
		}

		name, err := readNBTString(r)
		if err != nil {
			return fmt.Errorf("reading tag name: %w", err)
		}

		switch name {
		case "name":
			if tagType != 0x08 { // TAG_String
				return fmt.Errorf("'name' field: expected TAG_String, got %d", tagType)
			}
			s, err := readNBTString(r)
			if err != nil {
				return err
			}
			out.Name = s

		case "states":
			if tagType != 0x0A { // TAG_Compound
				return fmt.Errorf("'states' field: expected TAG_Compound, got %d", tagType)
			}
			states, err := readStatesCompound(r)
			if err != nil {
				return err
			}
			out.States = states

		case "version":
			if tagType != 0x03 { // TAG_Int
				if err := skipNBTValue(r, tagType); err != nil {
					return err
				}
				continue
			}
			var v int32
			if err := binary.Read(r, binary.LittleEndian, &v); err != nil {
				return err
			}
			out.Version = v

		default:
			// Skip unknown fields
			if err := skipNBTValue(r, tagType); err != nil {
				return fmt.Errorf("skipping field %q (type %d): %w", name, tagType, err)
			}
		}
	}
}

// readStatesCompound reads a TAG_Compound containing block state key-value pairs.
func readStatesCompound(r *bytes.Reader) (map[string]interface{}, error) {
	states := make(map[string]interface{})
	for {
		tagType, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		if tagType == 0x00 { // TAG_End
			return states, nil
		}
		name, err := readNBTString(r)
		if err != nil {
			return nil, err
		}
		val, err := readNBTScalar(r, tagType)
		if err != nil {
			return nil, fmt.Errorf("state %q: %w", name, err)
		}
		states[name] = val
	}
}

// readNBTScalar reads a scalar NBT value by tag type (no compound/list).
func readNBTScalar(r *bytes.Reader, tagType byte) (interface{}, error) {
	switch tagType {
	case 0x01: // TAG_Byte
		b, err := r.ReadByte()
		return int8(b), err
	case 0x02: // TAG_Short
		var v int16
		return v, binary.Read(r, binary.LittleEndian, &v)
	case 0x03: // TAG_Int
		var v int32
		return v, binary.Read(r, binary.LittleEndian, &v)
	case 0x04: // TAG_Long
		var v int64
		return v, binary.Read(r, binary.LittleEndian, &v)
	case 0x05: // TAG_Float
		var v float32
		return v, binary.Read(r, binary.LittleEndian, &v)
	case 0x06: // TAG_Double
		var v float64
		return v, binary.Read(r, binary.LittleEndian, &v)
	case 0x08: // TAG_String
		return readNBTString(r)
	default:
		if err := skipNBTValue(r, tagType); err != nil {
			return nil, err
		}
		return nil, nil
	}
}

// readNBTString reads a uint16-length-prefixed string (little-endian).
func readNBTString(r *bytes.Reader) (string, error) {
	var length uint16
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return "", err
	}
	if length == 0 {
		return "", nil
	}
	buf := make([]byte, length)
	if _, err := r.Read(buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

// skipNBTValue discards a value of the given tag type from r.
func skipNBTValue(r *bytes.Reader, tagType byte) error {
	switch tagType {
	case 0x01:
		_, err := r.ReadByte()
		return err
	case 0x02:
		_, err := r.Seek(2, 1)
		return err
	case 0x03, 0x05:
		_, err := r.Seek(4, 1)
		return err
	case 0x04, 0x06:
		_, err := r.Seek(8, 1)
		return err
	case 0x07: // TAG_Byte_Array
		var length int32
		if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
			return err
		}
		_, err := r.Seek(int64(length), 1)
		return err
	case 0x08: // TAG_String
		var length uint16
		if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
			return err
		}
		_, err := r.Seek(int64(length), 1)
		return err
	case 0x09: // TAG_List
		elemType, err := r.ReadByte()
		if err != nil {
			return err
		}
		var count int32
		if err := binary.Read(r, binary.LittleEndian, &count); err != nil {
			return err
		}
		for i := int32(0); i < count; i++ {
			if err := skipNBTValue(r, elemType); err != nil {
				return err
			}
		}
		return nil
	case 0x0A: // TAG_Compound
		for {
			t, err := r.ReadByte()
			if err != nil {
				return err
			}
			if t == 0x00 {
				return nil
			}
			if _, err := readNBTString(r); err != nil {
				return err
			}
			if err := skipNBTValue(r, t); err != nil {
				return err
			}
		}
	case 0x0B: // TAG_Int_Array
		var length int32
		if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
			return err
		}
		_, err := r.Seek(int64(length)*4, 1)
		return err
	case 0x0C: // TAG_Long_Array
		var length int32
		if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
			return err
		}
		_, err := r.Seek(int64(length)*8, 1)
		return err
	default:
		return fmt.Errorf("unknown tag type 0x%02X", tagType)
	}
}
