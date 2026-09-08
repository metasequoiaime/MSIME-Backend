package server

import "encoding/binary"

// 消耗上游转写额度前，先验证 RIFF 分块结构。
// 块长度不包含填充字节；fmt 和 data 为必需块，允许附加块。
func validWAV(audio []byte) bool {
	if len(audio) < 44 || string(audio[:4]) != "RIFF" || string(audio[8:12]) != "WAVE" ||
		uint64(binary.LittleEndian.Uint32(audio[4:8]))+8 != uint64(len(audio)) {
		return false
	}
	var format, dataSeen bool
	var align uint16
	var dataBytes uint32
	for offset := 12; offset < len(audio); {
		if len(audio)-offset < 8 {
			return false
		}
		name := string(audio[offset : offset+4])
		size := binary.LittleEndian.Uint32(audio[offset+4 : offset+8])
		offset += 8
		if uint64(size) > uint64(len(audio)-offset) {
			return false
		}
		chunk := audio[offset : offset+int(size)]
		switch name {
		case "fmt ":
			if format || size < 16 {
				return false
			}
			format = true
			encoding := binary.LittleEndian.Uint16(chunk[:2])
			channels := binary.LittleEndian.Uint16(chunk[2:4])
			rate := binary.LittleEndian.Uint32(chunk[4:8])
			byteRate := binary.LittleEndian.Uint32(chunk[8:12])
			align = binary.LittleEndian.Uint16(chunk[12:14])
			bits := binary.LittleEndian.Uint16(chunk[14:16])
			validBits := encoding == 1 && (bits == 8 || bits == 16 || bits == 24 || bits == 32) || encoding == 3 && (bits == 32 || bits == 64)
			if !validBits || channels < 1 || channels > 8 || rate < 8000 || rate > 192000 || align != channels*(bits/8) || uint64(byteRate) != uint64(rate)*uint64(align) {
				return false
			}
		case "data":
			if dataSeen || size == 0 {
				return false
			}
			dataSeen = true
			dataBytes = size
		}
		offset += int(size)
		if size%2 != 0 {
			if offset >= len(audio) {
				return false
			}
			offset++
		}
	}
	return format && dataSeen && align > 0 && dataBytes%uint32(align) == 0
}
