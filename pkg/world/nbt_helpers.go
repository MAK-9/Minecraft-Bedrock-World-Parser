package world

import (
	"bytes"
	"encoding/binary"
	"io"
)

func readInt16LE(r io.Reader) (int16, error) {
	var v int16
	return v, binary.Read(r, binary.LittleEndian, &v)
}

func readInt32LE(r io.Reader) (int32, error) {
	var v int32
	return v, binary.Read(r, binary.LittleEndian, &v)
}

func readInt64LE(r io.Reader) (int64, error) {
	var v int64
	return v, binary.Read(r, binary.LittleEndian, &v)
}

func readFloat32LE(r io.Reader) (float32, error) {
	var v float32
	return v, binary.Read(r, binary.LittleEndian, &v)
}

func readFloat64LE(r io.Reader) (float64, error) {
	var v float64
	return v, binary.Read(r, binary.LittleEndian, &v)
}

// buildNBTCompound wraps a map of fields into a Bedrock LE NBT compound byte slice.
// Used in tests and fixture generation.
func buildNBTCompound(fields map[string]nbtField) []byte {
	var buf bytes.Buffer
	buf.WriteByte(0x0A) // TAG_Compound
	writeUint16LE(&buf, 0) // empty name
	for name, f := range fields {
		buf.WriteByte(f.tagType)
		writeUint16LE(&buf, uint16(len(name)))
		buf.WriteString(name)
		buf.Write(f.data)
	}
	buf.WriteByte(0x00) // TAG_End
	return buf.Bytes()
}

type nbtField struct {
	tagType byte
	data    []byte
}

func nbtInt32Field(v int32) nbtField {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(v))
	return nbtField{tagType: 0x03, data: b}
}

func nbtInt64Field(v int64) nbtField {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(v))
	return nbtField{tagType: 0x04, data: b}
}

func nbtStringField(s string) nbtField {
	b := make([]byte, 2+len(s))
	binary.LittleEndian.PutUint16(b, uint16(len(s)))
	copy(b[2:], s)
	return nbtField{tagType: 0x08, data: b}
}

func nbtByteField(v int8) nbtField {
	return nbtField{tagType: 0x01, data: []byte{byte(v)}}
}

func nbtInt16Field(v int16) nbtField {
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, uint16(v))
	return nbtField{tagType: 0x02, data: b}
}

// nbtListField encodes a TAG_List (0x09) whose elements are compound payloads.
// Each entry in items must be a raw map payload — the fields + TAG_End with NO
// leading tag byte and NO name prefix (i.e. strip the first 3 bytes that
// buildNBTCompound prepends: 0x0A tag + 2-byte empty name).
func nbtListField(elemTagType byte, items [][]byte) nbtField {
	var buf bytes.Buffer
	buf.WriteByte(elemTagType)
	count := make([]byte, 4)
	binary.LittleEndian.PutUint32(count, uint32(len(items)))
	buf.Write(count)
	for _, payload := range items {
		buf.Write(payload)
	}
	return nbtField{tagType: 0x09, data: buf.Bytes()}
}

func writeUint16LE(buf *bytes.Buffer, v uint16) {
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, v)
	buf.Write(b)
}
