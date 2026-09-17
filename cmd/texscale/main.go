// Command texscale downscales the textures baked into a glTF binary.
//
//	go run ./cmd/texscale -src assets/models-src -out assets/models -size 512
//
// The models are authored with 2048x2048 base-colour and surface maps, which
// is the right thing to keep: a source that has been thrown away cannot be
// re-cut later, and the next screen is always bigger. It is the wrong thing to
// ship. A structure covers roughly a hundred pixels at the camera distance
// this game is played at, so a 2048 map is spending four megabytes to describe
// detail no one will see — and eight of them took the binary to seventy-two.
//
// So the hi-res models are the source, this is the bake, and the result is
// what gets embedded. The same shape as assets/icons-src -> cmd/iconatlas ->
// assets/icons.png, for the same reason.
//
// Geometry is copied through untouched. Only the images are re-encoded, which
// means the material names and base colours the game matches on — see
// internal/artcheck — come out the far side exactly as they went in.
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"sort"

	xdraw "golang.org/x/image/draw"
)

const (
	glbMagic   = 0x46546C67 // "glTF"
	glbVersion = 2
	chunkJSON  = 0x4E4F534A // "JSON"
	chunkBIN   = 0x004E4942 // "BIN\0"
)

// The parts of the glTF document this tool has to understand.
//
// Everything else rides through as json.RawMessage in a map, so a field this
// tool has never heard of is preserved rather than dropped — which matters,
// because a glTF carries extensions and a rewrite that silently discarded one
// would be worse than not rewriting at all.
type jsonBuffer struct {
	ByteLength int `json:"byteLength"`
}

type jsonBufferView struct {
	Buffer     int `json:"buffer"`
	ByteOffset int `json:"byteOffset,omitempty"`
	ByteLength int `json:"byteLength"`

	// Target and ByteStride are not used here but must survive the round
	// trip: a stride dropped from a vertex buffer view unpacks the mesh.
	ByteStride *int   `json:"byteStride,omitempty"`
	Target     *int   `json:"target,omitempty"`
	Name       string `json:"name,omitempty"`
}

type jsonImage struct {
	BufferView *int   `json:"bufferView,omitempty"`
	MimeType   string `json:"mimeType,omitempty"`
	Name       string `json:"name,omitempty"`
}

func main() {
	src := flag.String("src", "assets/models-src", "directory of authored models")
	out := flag.String("out", "assets/models", "directory to write shipped models to")
	size := flag.Int("size", 512, "longest texture edge to allow, in pixels")
	flag.Parse()

	entries, err := os.ReadDir(*src)
	if err != nil {
		log.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".glb" {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		log.Fatalf("no .glb files in %s", *src)
	}

	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}

	var before, after int64
	for _, name := range names {
		in := filepath.Join(*src, name)
		dst := filepath.Join(*out, name)

		inSize, outSize, notes, err := scale(in, dst, *size)
		if err != nil {
			log.Fatalf("%s: %v", name, err)
		}
		before += inSize
		after += outSize

		fmt.Printf("%-18s %6.1f MB -> %5.2f MB   %s\n",
			name, mb(inSize), mb(outSize), notes)
	}

	fmt.Printf("\n%d models: %.1f MB -> %.1f MB (%.0f%% smaller)\n",
		len(names), mb(before), mb(after), 100*(1-float64(after)/float64(before)))
}

func mb(n int64) float64 { return float64(n) / (1 << 20) }

// scale rewrites one model with its textures capped at max pixels on a side.
func scale(inPath, outPath string, max int) (inSize, outSize int64, notes string, err error) {
	blob, err := os.ReadFile(inPath)
	if err != nil {
		return 0, 0, "", err
	}
	inSize = int64(len(blob))

	jsonChunk, binChunk, err := splitGLB(blob)
	if err != nil {
		return 0, 0, "", err
	}

	// Decode into a map first so unknown fields survive, then pull out the
	// three arrays this tool changes.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(jsonChunk, &raw); err != nil {
		return 0, 0, "", fmt.Errorf("parse JSON chunk: %w", err)
	}

	var views []jsonBufferView
	if v, ok := raw["bufferViews"]; ok {
		if err := json.Unmarshal(v, &views); err != nil {
			return 0, 0, "", fmt.Errorf("bufferViews: %w", err)
		}
	}
	var images []jsonImage
	if v, ok := raw["images"]; ok {
		if err := json.Unmarshal(v, &images); err != nil {
			return 0, 0, "", fmt.Errorf("images: %w", err)
		}
	}

	// The new bytes for each bufferView, indexed as the views are. Views not
	// touched below keep their original slice.
	payload := make([][]byte, len(views))
	for i, v := range views {
		if v.ByteOffset+v.ByteLength > len(binChunk) {
			return 0, 0, "", fmt.Errorf("bufferView %d runs past the binary chunk", i)
		}
		payload[i] = binChunk[v.ByteOffset : v.ByteOffset+v.ByteLength]
	}

	resized := 0
	for _, im := range images {
		if im.BufferView == nil {
			continue // an image referenced by URI, which a .glb should not have
		}
		idx := *im.BufferView
		if idx < 0 || idx >= len(payload) {
			return 0, 0, "", fmt.Errorf("image %q names bufferView %d", im.Name, idx)
		}

		small, changed, err := downscalePNG(payload[idx], max)
		if err != nil {
			return 0, 0, "", fmt.Errorf("image %q: %w", im.Name, err)
		}
		if changed {
			payload[idx] = small
			resized++
		}
	}

	// Rebuild the binary chunk with the new lengths, keeping view order. glTF
	// wants each view 4-byte aligned, and an accessor reading from a
	// misaligned offset is undefined rather than merely slow.
	var bin bytes.Buffer
	for i := range views {
		for bin.Len()%4 != 0 {
			bin.WriteByte(0)
		}
		views[i].ByteOffset = bin.Len()
		views[i].ByteLength = len(payload[i])
		bin.Write(payload[i])
	}
	for bin.Len()%4 != 0 {
		bin.WriteByte(0)
	}

	// A single buffer, which is what a .glb has by definition.
	var buffers []jsonBuffer
	if v, ok := raw["buffers"]; ok {
		if err := json.Unmarshal(v, &buffers); err != nil {
			return 0, 0, "", fmt.Errorf("buffers: %w", err)
		}
	}
	if len(buffers) != 1 {
		return 0, 0, "", fmt.Errorf("expected one buffer, found %d", len(buffers))
	}
	buffers[0].ByteLength = bin.Len()

	if raw["bufferViews"], err = json.Marshal(views); err != nil {
		return 0, 0, "", err
	}
	if raw["buffers"], err = json.Marshal(buffers); err != nil {
		return 0, 0, "", err
	}

	newJSON, err := json.Marshal(raw)
	if err != nil {
		return 0, 0, "", err
	}

	packed := packGLB(newJSON, bin.Bytes())
	if err := os.WriteFile(outPath, packed, 0o644); err != nil {
		return 0, 0, "", err
	}

	notes = fmt.Sprintf("%d texture(s) resized", resized)
	if resized == 0 {
		notes = "already within size"
	}
	return inSize, int64(len(packed)), notes, nil
}

// downscalePNG re-encodes an image with its longest edge capped, reporting
// whether it actually changed.
func downscalePNG(data []byte, max int) ([]byte, bool, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, false, err
	}

	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= max && h <= max {
		return data, false, nil
	}

	nw, nh := w, h
	if w >= h {
		nw, nh = max, h*max/w
	} else {
		nw, nh = w*max/h, max
	}

	// CatmullRom, as the icon atlas uses: these are being reduced four to one,
	// and a box filter turns the painted panel lines into aliasing that
	// crawls as the camera moves.
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, xdraw.Src, nil)

	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, dst); err != nil {
		return nil, false, err
	}
	return buf.Bytes(), true, nil
}

// splitGLB returns the JSON and binary chunks of a .glb.
func splitGLB(blob []byte) (jsonChunk, binChunk []byte, err error) {
	if len(blob) < 12 || binary.LittleEndian.Uint32(blob) != glbMagic {
		return nil, nil, fmt.Errorf("not a glTF binary")
	}
	if v := binary.LittleEndian.Uint32(blob[4:]); v != glbVersion {
		return nil, nil, fmt.Errorf("glTF version %d, want %d", v, glbVersion)
	}

	for off := 12; off+8 <= len(blob); {
		length := int(binary.LittleEndian.Uint32(blob[off:]))
		kind := binary.LittleEndian.Uint32(blob[off+4:])
		body := off + 8
		if body+length > len(blob) {
			return nil, nil, fmt.Errorf("chunk runs past the end of the file")
		}
		switch kind {
		case chunkJSON:
			jsonChunk = blob[body : body+length]
		case chunkBIN:
			binChunk = blob[body : body+length]
		}
		off = body + length
	}
	if jsonChunk == nil {
		return nil, nil, fmt.Errorf("no JSON chunk")
	}
	return jsonChunk, binChunk, nil
}

// packGLB assembles a .glb. The JSON chunk is padded with spaces and the
// binary one with zeros, which is what the specification asks for — a decoder
// is entitled to assume the padding is those bytes.
func packGLB(jsonChunk, binChunk []byte) []byte {
	pad := func(b []byte, with byte) []byte {
		for len(b)%4 != 0 {
			b = append(b, with)
		}
		return b
	}
	jsonChunk = pad(jsonChunk, ' ')
	binChunk = pad(binChunk, 0)

	total := 12 + 8 + len(jsonChunk)
	if len(binChunk) > 0 {
		total += 8 + len(binChunk)
	}

	out := make([]byte, 0, total)
	be := binary.LittleEndian
	u32 := func(v uint32) {
		var b [4]byte
		be.PutUint32(b[:], v)
		out = append(out, b[:]...)
	}

	u32(glbMagic)
	u32(glbVersion)
	u32(uint32(total))

	u32(uint32(len(jsonChunk)))
	u32(chunkJSON)
	out = append(out, jsonChunk...)

	if len(binChunk) > 0 {
		u32(uint32(len(binChunk)))
		u32(chunkBIN)
		out = append(out, binChunk...)
	}
	return out
}
