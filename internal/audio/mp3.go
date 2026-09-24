package audio

import (
	"encoding/binary"

	"github.com/dodopok/estevao-api-go/internal/rb"
)

const mp3MaxBytes = 64 << 20

var (
	bitratesV1  = [16]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}
	bitratesV2  = [16]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0}
	sampleRates = map[uint32][3]int{3: {44100, 48000, 32000}, 2: {22050, 24000, 16000}, 0: {11025, 12000, 8000}}
	frameSizes  = map[uint32]int{3: 1152, 2: 576, 0: 576}
)

// Mp3Duration ports Audio::Mp3Duration.call on the file's bytes: the playing
// time from the Layer III frame headers, rounded to milliseconds, or nil.
func Mp3Duration(data []byte) *float64 {
	if len(data) > mp3MaxBytes {
		return nil
	}
	offset := 0
	if len(data) >= 3 && string(data[:3]) == "ID3" && len(data) >= 10 {
		size := 0
		for _, b := range data[6:10] {
			size = size<<7 | int(b&0x7F)
		}
		offset = 10 + size
	}
	total, frames := 0.0, 0
	for offset < len(data)-4 {
		header := binary.BigEndian.Uint32(data[offset : offset+4])
		length, samples, rate, ok := mp3Frame(header)
		if !ok {
			offset++
			continue
		}
		total += float64(samples) / float64(rate)
		frames++
		offset += length
	}
	if frames == 0 {
		return nil
	}
	v := rb.RoundFloat(total, 3)
	return &v
}

func mp3Frame(header uint32) (length, samples, rate int, ok bool) {
	if (header>>16)&0xFFE0 != 0xFFE0 || (header>>17)&0x03 != 1 {
		return 0, 0, 0, false
	}
	version := (header >> 19) & 0x03
	rates, known := sampleRates[version]
	if !known {
		return 0, 0, 0, false
	}
	rateIndex := (header >> 10) & 0x03
	if rateIndex == 3 {
		return 0, 0, 0, false
	}
	table := bitratesV2
	if version == 3 {
		table = bitratesV1
	}
	bitrate := table[(header>>12)&0x0F]
	if bitrate == 0 {
		return 0, 0, 0, false
	}
	rate = rates[rateIndex]
	samples = frameSizes[version]
	length = (samples/8)*bitrate*1000/rate + int((header>>9)&0x01)
	if length <= 4 {
		return 0, 0, 0, false
	}
	return length, samples, rate, true
}
