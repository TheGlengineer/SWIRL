package main

// Menu music: converts the owner's WAV or MP3 file into BGM.ADP, the format SWIRL (and the maintained
// openMenu fork) stream from the menu disc: a 32 byte "OMBG" header, then Yamaha AICA 4 bit ADPCM at
// 44.1 kHz. Stereo packs one frame per byte (left in the high nibble), mono packs two samples per byte.

import (
	"bytes"
	"crypto/sha1"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	mp3 "github.com/hajimehoshi/go-mp3"
)

const (
	bgmRate       = 44100
	bgmMaxSeconds = 20 * 60
)

func musicPath(root string) string { return filepath.Join(root, editsDir, "BGM.ADP") }

type pcm struct {
	rate     int
	channels int
	samples  []float32 // interleaved, -1..1
}

func decodeWAV(b []byte) (*pcm, error) {
	if len(b) < 44 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return nil, errors.New("not a WAV file")
	}
	var format, channels, bits int
	var rate int
	var data []byte
	for p := 12; p+8 <= len(b); {
		id := string(b[p : p+4])
		n := int(binary.LittleEndian.Uint32(b[p+4:]))
		body := b[p+8:]
		if n > len(body) {
			n = len(body)
		}
		switch id {
		case "fmt ":
			if n < 16 {
				return nil, errors.New("broken WAV header")
			}
			format = int(binary.LittleEndian.Uint16(body[0:]))
			channels = int(binary.LittleEndian.Uint16(body[2:]))
			rate = int(binary.LittleEndian.Uint32(body[4:]))
			bits = int(binary.LittleEndian.Uint16(body[14:]))
			if format == 0xFFFE && n >= 26 { // WAVE_FORMAT_EXTENSIBLE
				format = int(binary.LittleEndian.Uint16(body[24:]))
			}
		case "data":
			data = body[:n]
		}
		p += 8 + n + n%2
	}
	if data == nil || channels < 1 || rate < 4000 {
		return nil, errors.New("this WAV file has no audio SWIRL can read")
	}
	out := &pcm{rate: rate, channels: channels}
	switch {
	case format == 1 && bits == 16:
		for i := 0; i+2 <= len(data); i += 2 {
			out.samples = append(out.samples, float32(int16(binary.LittleEndian.Uint16(data[i:])))/32768)
		}
	case format == 1 && bits == 8:
		for _, v := range data {
			out.samples = append(out.samples, (float32(v)-128)/128)
		}
	case format == 1 && bits == 24:
		for i := 0; i+3 <= len(data); i += 3 {
			v := int32(uint32(data[i])<<8|uint32(data[i+1])<<16|uint32(data[i+2])<<24) >> 8
			out.samples = append(out.samples, float32(v)/8388608)
		}
	case format == 3 && bits == 32:
		for i := 0; i+4 <= len(data); i += 4 {
			out.samples = append(out.samples, math.Float32frombits(binary.LittleEndian.Uint32(data[i:])))
		}
	default:
		return nil, fmt.Errorf("WAV files with %d bit format %d are not supported; save it as 16 bit PCM", bits, format)
	}
	return out, nil
}

func decodeMP3(r io.Reader) (*pcm, error) {
	d, err := mp3.NewDecoder(r)
	if err != nil {
		return nil, fmt.Errorf("could not read the MP3: %v", err)
	}
	raw, err := io.ReadAll(d)
	if err != nil && len(raw) == 0 {
		return nil, fmt.Errorf("could not decode the MP3: %v", err)
	}
	out := &pcm{rate: d.SampleRate(), channels: 2, samples: make([]float32, len(raw)/2)}
	for i := range out.samples {
		out.samples[i] = float32(int16(binary.LittleEndian.Uint16(raw[i*2:]))) / 32768
	}
	return out, nil
}

// resample converts to 44.1 kHz with linear interpolation and keeps at most two channels.
func (p *pcm) toStereo44k() (left, right []float32) {
	frames := len(p.samples) / p.channels
	get := func(f, c int) float32 {
		if c >= p.channels {
			c = 0
		}
		return p.samples[f*p.channels+c]
	}
	outN := int(int64(frames) * bgmRate / int64(p.rate))
	left = make([]float32, outN)
	right = make([]float32, outN)
	for i := 0; i < outN; i++ {
		pos := float64(i) * float64(p.rate) / bgmRate
		f := int(pos)
		t := float32(pos - float64(f))
		g := f + 1
		if g >= frames {
			g = frames - 1
		}
		left[i] = get(f, 0)*(1-t) + get(g, 0)*t
		right[i] = get(f, 1)*(1-t) + get(g, 1)*t
	}
	return
}

// AICA ADPCM (the same algorithm as ffmpeg's adpcm_yamaha)
type aicaEnc struct{ pred, step int }

var aicaScale = [16]int{230, 230, 230, 230, 307, 409, 512, 614, 230, 230, 230, 230, 307, 409, 512, 614}
var aicaDiff = [16]int{1, 3, 5, 7, 9, 11, 13, 15, -1, -3, -5, -7, -9, -11, -13, -15}

func (e *aicaEnc) encode(s int) int {
	if e.step == 0 {
		e.step = 127
	}
	delta := s - e.pred
	a := delta
	if a < 0 {
		a = -a
	}
	n := a * 4 / e.step
	if n > 7 {
		n = 7
	}
	if delta < 0 {
		n += 8
	}
	e.pred += e.step * aicaDiff[n] / 8
	if e.pred > 32767 {
		e.pred = 32767
	} else if e.pred < -32768 {
		e.pred = -32768
	}
	e.step = e.step * aicaScale[n] >> 8
	if e.step < 127 {
		e.step = 127
	} else if e.step > 24576 {
		e.step = 24576
	}
	return n
}

// loopTail returns nibbles that take an encoder from its current state back to predictor 0, step 127
// (the state a fresh decoder starts in), with a length whose parity is wantParity, so the padded track
// can be finished with +15/-15 pairs that keep that state.
func loopTail(e *aicaEnc, wantParity int) []byte {
	var out []byte
	for i := 0; i < 4000 && (e.step != 127 || e.pred > 3000 || e.pred < -3000); i++ {
		out = append(out, byte(e.encode(0)))
	}
	type node struct{ pred, step, parity int }
	start := node{e.pred, e.step, len(out) % 2}
	target := node{0, 127, wantParity}
	prev := map[node]node{start: start}
	how := map[node]byte{}
	queue := []node{start}
	found := start == target
	for len(queue) > 0 && !found {
		n := queue[0]
		queue = queue[1:]
		for m := 0; m < 16; m++ {
			p := n.pred + n.step*aicaDiff[m]/8
			st := n.step * aicaScale[m] >> 8
			if st < 127 {
				st = 127
			}
			if p > 4000 || p < -4000 || st > 600 {
				continue
			}
			nx := node{p, st, 1 - n.parity}
			if _, seen := prev[nx]; seen {
				continue
			}
			prev[nx] = n
			how[nx] = byte(m)
			if nx == target {
				found = true
				break
			}
			queue = append(queue, nx)
		}
	}
	if !found {
		return out
	}
	var path []byte
	for n := target; n != start; n = prev[n] {
		path = append([]byte{how[n]}, path...)
	}
	e.pred, e.step = 0, 127
	return append(out, path...)
}

func toPCM16(v float32) int {
	x := int(math.Round(float64(v) * 32767))
	if x > 32767 {
		return 32767
	}
	if x < -32768 {
		return -32768
	}
	return x
}

// encodeBGM builds a stereo BGM.ADP. gain lets the owner even out loud or quiet tracks.
func encodeBGM(p *pcm) ([]byte, float64, error) {
	l, r := p.toStereo44k()
	if len(l) == 0 {
		return nil, 0, errors.New("the file contains no audio")
	}
	secs := float64(len(l)) / bgmRate
	if secs > bgmMaxSeconds {
		return nil, 0, fmt.Errorf("the track is %.0f minutes long; menu music can be up to 20 minutes", secs/60)
	}
	frames := len(l)
	var le, re aicaEnc
	lnib := make([]byte, 0, frames+4096)
	rnib := make([]byte, 0, frames+4096)
	for i := 0; i < frames; i++ {
		lnib = append(lnib, byte(le.encode(toPCM16(l[i]))))
		rnib = append(rnib, byte(re.encode(toPCM16(r[i]))))
	}
	// The sound chip keeps its decoder state when the track loops, but the track was encoded from a fresh
	// state. Unless both match, every loop adds an offset and the music gets louder until it distorts.
	// So the track ends with a short silent tail that brings both channels back to the starting state.
	want := frames % 2 // frames + tail must be even
	lt, rt := loopTail(&le, want), loopTail(&re, want)
	for len(lt) < len(rt) {
		lt = append(lt, 0, 8) // +15 then -15: stays at the start state
	}
	for len(rt) < len(lt) {
		rt = append(rt, 0, 8)
	}
	lnib = append(lnib, lt...)
	rnib = append(rnib, rt...)
	for len(lnib)%32 != 0 && len(lnib)%2 == 0 {
		lnib = append(lnib, 0, 8)
		rnib = append(rnib, 0, 8)
	}
	payload := make([]byte, len(lnib))
	for i := range payload {
		payload[i] = lnib[i]<<4 | rnib[i]
	}
	var hdr bytes.Buffer
	hdr.WriteString("OMBG")
	binary.Write(&hdr, binary.LittleEndian, uint32(1))
	binary.Write(&hdr, binary.LittleEndian, uint32(bgmRate))
	binary.Write(&hdr, binary.LittleEndian, uint16(2))
	binary.Write(&hdr, binary.LittleEndian, uint16(0))
	binary.Write(&hdr, binary.LittleEndian, uint32(0))
	binary.Write(&hdr, binary.LittleEndian, uint32(len(payload)))
	binary.Write(&hdr, binary.LittleEndian, uint64(0))
	return append(hdr.Bytes(), payload...), secs, nil
}

// SetMusic converts a music file for the card. It is added to the menu disc at the next install.
func SetMusic(root, src string, log Logger) (float64, error) {
	b, err := os.ReadFile(src)
	if err != nil {
		return 0, fmt.Errorf("could not open %s", src)
	}
	var p *pcm
	switch strings.ToLower(filepath.Ext(src)) {
	case ".wav":
		p, err = decodeWAV(b)
	case ".mp3":
		p, err = decodeMP3(bytes.NewReader(b))
	case ".adp":
		if len(b) > 32 && string(b[:4]) == "OMBG" {
			os.MkdirAll(filepath.Join(root, editsDir), 0o755)
			os.Remove(themeMarker(root))
			return 0, os.WriteFile(musicPath(root), b, 0o644)
		}
		err = errors.New("that .ADP file is not menu music")
	default:
		err = errors.New("pick a .wav or .mp3 file")
	}
	if err != nil {
		return 0, err
	}
	out, secs, err := encodeBGM(p)
	if err != nil {
		return 0, err
	}
	os.MkdirAll(filepath.Join(root, editsDir), 0o755)
	os.Remove(filepath.Join(root, editsDir, "BGM.NONE"))
	os.Remove(themeMarker(root))
	if err := os.WriteFile(musicPath(root), out, 0o644); err != nil {
		return 0, err
	}
	log("Converted %s: %d:%02d of music, %.1f MB", filepath.Base(src), int(secs)/60, int(secs)%60, float64(len(out))/(1<<20))
	return secs, nil
}

// RemoveMusic drops the owner's music, so the SWIRL theme plays again. The theme itself cannot be removed;
// music can still be turned off on the Dreamcast in SWIRL's System tab.
func RemoveMusic(root string) error {
	os.Remove(filepath.Join(root, editsDir, "BGM.NONE")) // from versions that allowed no music at all
	// remembers the choice, so music left on the old menu disc is not picked up as theirs again
	os.MkdirAll(filepath.Join(root, editsDir), 0o755)
	os.WriteFile(themeMarker(root), []byte("use the SWIRL theme"), 0o644)
	err := os.Remove(musicPath(root))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ---------- the SWIRL theme ----------

// The theme ships inside the app as an MP3 (half the size of the converted file) and is converted once,
// then kept with the app's other data.

//go:embed assets/default_bgm.mp3
var defaultBGMmp3 []byte

const defaultMusicName = "TopStrike Main Theme"

var defaultBGMmu sync.Mutex

func defaultBGMPath() string {
	sum := sha1.Sum(defaultBGMmp3)
	return filepath.Join(appDataDir(), fmt.Sprintf("theme_%x.adp", sum[:6]))
}

// defaultMusic returns the SWIRL theme as BGM.ADP.
func defaultMusic() ([]byte, error) {
	defaultBGMmu.Lock()
	defer defaultBGMmu.Unlock()
	p := defaultBGMPath()
	if b, err := os.ReadFile(p); err == nil && len(b) > 32 && string(b[:4]) == "OMBG" {
		return b, nil
	}
	pc, err := decodeMP3(bytes.NewReader(defaultBGMmp3))
	if err != nil {
		return nil, fmt.Errorf("the SWIRL theme could not be read: %v", err)
	}
	out, _, err := encodeBGM(pc)
	if err != nil {
		return nil, err
	}
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, out, 0o644)
	return out, nil
}

func themeMarker(root string) string { return filepath.Join(root, editsDir, "BGM.THEME") }

func isDefaultMusic(b []byte) bool {
	d, err := defaultMusic()
	return err == nil && bytes.Equal(b, d)
}

type MusicInfo struct {
	Present     bool    `json:"present"` // the owner's own music is set
	Seconds     float64 `json:"seconds"`
	Bytes       int64   `json:"bytes"`
	DefaultName string  `json:"defaultName"`
	DefaultSecs float64 `json:"defaultSeconds"`
}

func GetMusicInfo(root string) MusicInfo {
	mi := MusicInfo{DefaultName: defaultMusicName, DefaultSecs: 201}
	st, err := os.Stat(musicPath(root))
	if err != nil {
		return mi
	}
	mi.Present, mi.Bytes, mi.Seconds = true, st.Size(), float64(st.Size()-32)/bgmRate
	return mi
}
