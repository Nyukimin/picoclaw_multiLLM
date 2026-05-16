package main

import (
	"os"
	"strconv"
	"strings"
	"time"

	sttinfra "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/stt"
)

func sttFinalTimeoutFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv("STT_FINAL_TIMEOUT_MS"))
	if raw == "" {
		return 1200 * time.Millisecond
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms < 200 {
		return 1200 * time.Millisecond
	}
	return time.Duration(ms) * time.Millisecond
}

func sttSilenceAbsThresholdFromEnv() int {
	raw := strings.TrimSpace(os.Getenv("STT_SILENCE_ABS_THRESHOLD"))
	if raw == "" {
		return 220
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 1 {
		return 220
	}
	return v
}

func isLikelySilentWAV(wav []byte, absThreshold int) bool {
	if len(wav) <= 44 {
		return false
	}
	if string(wav[0:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		return false
	}
	sampleBytes := wav[44:]
	if len(sampleBytes) < 2 {
		return false
	}
	var sum int64
	var n int64
	for i := 0; i+1 < len(sampleBytes); i += 2 {
		s := int16(sampleBytes[i]) | int16(sampleBytes[i+1])<<8
		if s < 0 {
			sum += int64(-s)
		} else {
			sum += int64(s)
		}
		n++
	}
	if n == 0 {
		return false
	}
	avgAbs := int(sum / n)
	return avgAbs < absThreshold
}

func normalizeSTTAudioPayload(payload []byte) []byte {
	if sttinfra.IsWAV(payload) {
		return payload
	}
	if len(payload) < 2 {
		return payload
	}
	audioLen := len(payload)
	if audioLen%2 != 0 {
		audioLen--
	}
	return pcm16LEToWAV(payload[:audioLen], 16000)
}

func pcm16LEToWAV(pcm []byte, sampleRate int) []byte {
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	dataSize := len(pcm)
	out := make([]byte, 44+dataSize)
	copy(out[0:4], "RIFF")
	putLE32(out[4:8], uint32(36+dataSize))
	copy(out[8:12], "WAVE")
	copy(out[12:16], "fmt ")
	putLE32(out[16:20], 16)
	putLE16(out[20:22], 1)
	putLE16(out[22:24], 1)
	putLE32(out[24:28], uint32(sampleRate))
	putLE32(out[28:32], uint32(sampleRate*2))
	putLE16(out[32:34], 2)
	putLE16(out[34:36], 16)
	copy(out[36:40], "data")
	putLE32(out[40:44], uint32(dataSize))
	copy(out[44:], pcm)
	return out
}

func putLE16(dst []byte, v uint16) {
	dst[0] = byte(v)
	dst[1] = byte(v >> 8)
}

func putLE32(dst []byte, v uint32) {
	dst[0] = byte(v)
	dst[1] = byte(v >> 8)
	dst[2] = byte(v >> 16)
	dst[3] = byte(v >> 24)
}
