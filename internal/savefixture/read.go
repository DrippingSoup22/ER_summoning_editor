package savefixture

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf16"

	"github.com/klauspost/compress/zstd"
)

// Reader decodes a decrypted PS4 save independently of internal/savecore, so a
// test can check what the product actually wrote rather than trusting the
// product's own report of what it wrote.
type Reader struct {
	version uint32
	binder  []byte
}

// Open decrypts and decompresses a save's embedded regulation.
func Open(save []byte) (*Reader, error) {
	if len(save) < UserData11Offset+userdata11HeaderSize+aesIVSize {
		return nil, fmt.Errorf("save is %d bytes, too small to hold UserData11", len(save))
	}
	userdata11 := save[UserData11Offset:]
	if string(userdata11[:4]) != " GER" {
		return nil, fmt.Errorf("UserData11 magic is % X, not \" GER\"", userdata11[:4])
	}
	iv := userdata11[userdata11HeaderSize : userdata11HeaderSize+aesIVSize]
	ciphertext := userdata11[userdata11HeaderSize+aesIVSize:]
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext length %d is not block-aligned", len(ciphertext))
	}
	block, err := aes.NewCipher(regulationKey)
	if err != nil {
		return nil, err
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plaintext, ciphertext)
	if len(plaintext) < dcxHeaderSize || string(plaintext[:4]) != "DCX\x00" {
		return nil, fmt.Errorf("decrypted block is not DCX")
	}
	compressed := int(binary.BigEndian.Uint32(plaintext[32:36]))
	if compressed <= 0 || dcxHeaderSize+compressed > len(plaintext) {
		return nil, fmt.Errorf("DCX declares %d compressed bytes, which does not fit", compressed)
	}
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		return nil, err
	}
	defer decoder.Close()
	binder, err := decoder.DecodeAll(plaintext[dcxHeaderSize:dcxHeaderSize+compressed], nil)
	if err != nil {
		return nil, fmt.Errorf("decompress regulation: %w", err)
	}
	if string(binder[:4]) != "BND4" {
		return nil, fmt.Errorf("regulation is not BND4")
	}
	return &Reader{version: binary.LittleEndian.Uint32(userdata11[8:12]), binder: binder}, nil
}

// RegulationVersion is the version word in the UserData11 header.
func (reader *Reader) RegulationVersion() uint32 { return reader.version }

// Row returns a copy of one PARAM row's bytes.
func (reader *Reader) Row(entry string, rowID int32) ([]byte, error) {
	rows, blob, stride, err := reader.table(entry)
	if err != nil {
		return nil, err
	}
	offset, present := rows[rowID]
	if !present {
		return nil, fmt.Errorf("%s has no row %d", entry, rowID)
	}
	return append([]byte(nil), blob[offset:offset+stride]...), nil
}

// RowIDs lists a table's rows in file order.
func (reader *Reader) RowIDs(entry string) ([]int32, error) {
	blob, err := reader.entryBytes(entry)
	if err != nil {
		return nil, err
	}
	ordered, _, err := parseRows(blob)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", entry, err)
	}
	ids := make([]int32, 0, len(ordered))
	for _, row := range ordered {
		ids = append(ids, row.id)
	}
	return ids, nil
}

// GoodsTrigger reports refId_default and enable_multi for one goods row.
func (reader *Reader) GoodsTrigger(rowID int32) (trigger int32, enableMulti bool, err error) {
	row, err := reader.Row("EquipParamGoods.param", rowID)
	if err != nil {
		return 0, false, err
	}
	return int32(binary.LittleEndian.Uint32(row[goodsRefIDDefaultOffset:])),
		row[goodsFlagsOffset]&goodsEnableMultiMask != 0, nil
}

// GoodsPrice reports consumeMP and consumeHP for one goods row: what a summon
// costs, read independently of savecore so a test can check the writer against
// something that is not the writer.
func (reader *Reader) GoodsPrice(rowID int32) (fp, hp int32, err error) {
	row, err := reader.Row("EquipParamGoods.param", rowID)
	if err != nil {
		return 0, 0, err
	}
	return int32(int16(binary.LittleEndian.Uint16(row[goodsConsumeMPOffset:]))),
		int32(int16(binary.LittleEndian.Uint16(row[goodsConsumeHPOffset:]))), nil
}

// BuddyAppearsOnMonument reports appearOnAroundSekihi for one BuddyParam row.
func (reader *Reader) BuddyAppearsOnMonument(rowID int32) (bool, error) {
	row, err := reader.Row("BuddyParam.param", rowID)
	if err != nil {
		return false, err
	}
	return row[buddyAppearOffset] != 0, nil
}

// StoneField reads one named expanded-area field from a BuddyStoneParam row.
func (reader *Reader) StoneField(rowID int32, name string) (int64, error) {
	field, known := StoneFieldOffsets[name]
	if !known {
		return 0, fmt.Errorf("%q is not an expanded-area field", name)
	}
	row, err := reader.Row("BuddyStoneParam.param", rowID)
	if err != nil {
		return 0, err
	}
	if field.Bits == 16 {
		// overwriteReturnRange is signed and its cleared value is -1, so the
		// 16-bit read must sign-extend or the check below reads 65535.
		if name == "overwriteReturnRange" {
			return int64(int16(binary.LittleEndian.Uint16(row[field.Offset:]))), nil
		}
		return int64(binary.LittleEndian.Uint16(row[field.Offset:])), nil
	}
	return int64(binary.LittleEndian.Uint32(row[field.Offset:])), nil
}

func (reader *Reader) table(entry string) (map[int32]int, []byte, int, error) {
	blob, err := reader.entryBytes(entry)
	if err != nil {
		return nil, nil, 0, err
	}
	ordered, byID, err := parseRows(blob)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("%s: %w", entry, err)
	}
	if len(ordered) < 2 {
		return nil, nil, 0, fmt.Errorf("%s has too few rows to derive a stride", entry)
	}
	stride := ordered[1].offset - ordered[0].offset
	if stride <= 0 {
		return nil, nil, 0, fmt.Errorf("%s has a non-positive stride", entry)
	}
	return byID, blob, stride, nil
}

func (reader *Reader) entryBytes(name string) ([]byte, error) {
	count := int(binary.LittleEndian.Uint32(reader.binder[0x0C:0x10]))
	for index := 0; index < count; index++ {
		base := binderHeaderSize + index*binderEntrySize
		size := int(binary.LittleEndian.Uint64(reader.binder[base+8 : base+16]))
		offset := int(binary.LittleEndian.Uint32(reader.binder[base+24 : base+28]))
		nameOffset := int(binary.LittleEndian.Uint32(reader.binder[base+32 : base+36]))
		entryName, err := decodeUTF16(reader.binder, nameOffset)
		if err != nil {
			return nil, err
		}
		if entryName == name || strings.HasSuffix(entryName, `\`+name) || strings.HasSuffix(entryName, "/"+name) {
			if offset < 0 || size < 0 || offset+size > len(reader.binder) {
				return nil, fmt.Errorf("entry %q lies outside the binder", name)
			}
			return reader.binder[offset : offset+size], nil
		}
	}
	return nil, fmt.Errorf("no binder entry named %q", name)
}

type rowLocation struct {
	id     int32
	offset int
}

// parseRows walks the header the same way the game does, but written out
// separately from the product so the two can disagree visibly.
func parseRows(blob []byte) ([]rowLocation, map[int32]int, error) {
	if len(blob) < paramHeaderSize {
		return nil, nil, fmt.Errorf("PARAM is %d bytes, shorter than its header", len(blob))
	}
	tableStart, entrySize, _ := paramLayout(blob[0x2D])
	rowCount := int(binary.LittleEndian.Uint16(blob[10:12]))
	if tableStart+rowCount*entrySize > len(blob) {
		return nil, nil, fmt.Errorf("row table of %d rows overruns the entry", rowCount)
	}
	ordered := make([]rowLocation, 0, rowCount)
	byID := make(map[int32]int, rowCount)
	for index := 0; index < rowCount; index++ {
		base := tableStart + index*entrySize
		id := int32(binary.LittleEndian.Uint32(blob[base : base+4]))
		var offset int
		if entrySize == 24 {
			offset = int(binary.LittleEndian.Uint64(blob[base+8 : base+16]))
		} else {
			offset = int(binary.LittleEndian.Uint32(blob[base+4 : base+8]))
		}
		if offset < 0 || offset > len(blob) {
			return nil, nil, fmt.Errorf("row %d points outside the entry", id)
		}
		ordered = append(ordered, rowLocation{id: id, offset: offset})
		byID[id] = offset
	}
	return ordered, byID, nil
}

func decodeUTF16(blob []byte, offset int) (string, error) {
	if offset < 0 || offset >= len(blob) {
		return "", fmt.Errorf("name offset 0x%X lies outside the binder", offset)
	}
	var units []uint16
	for position := offset; position+2 <= len(blob); position += 2 {
		unit := binary.LittleEndian.Uint16(blob[position : position+2])
		if unit == 0 {
			return string(utf16.Decode(units)), nil
		}
		units = append(units, unit)
	}
	return "", fmt.Errorf("unterminated name at 0x%X", offset)
}
