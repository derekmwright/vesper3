// Package artcheck reads the material list out of a glTF binary.
//
// It exists because several structures are animated by *material*: the game
// finds the battery's charge strips, the condenser's fin band and the
// greenhouse's grow lamps by matching the base colour a primitive came in
// with, then drives that primitive's emission from the simulation. Nothing in
// the file says "this is the charge indicator" — renderer.ModelMesh does not
// carry the material name — so an exact colour is the only handle there is.
//
// That makes the art a contract, and an undefended one. Re-exporting a model
// with a slightly different colour, or with its indicator merged into the body
// material, does not fail to load and does not warn. The strip simply stops
// lighting up, months later, in a build nobody connects to the export.
//
// So the colours are asserted: against the shipped models in a test, and
// against a freshly built one in cmd/modelcheck before it is allowed to
// replace what is there.
package artcheck

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
)

// Material is one glTF material, reduced to what the game matches on.
type Material struct {
	Name      string
	BaseColor [3]float32
}

// glTF binary container constants.
const (
	magic     = 0x46546C67 // "glTF"
	chunkJSON = 0x4E4F534A // "JSON"
)

// Materials returns the materials in a .glb, in file order.
//
// Only the JSON chunk is read. The binary chunk holds geometry, which is not
// what is being checked and is the large half of the file.
func Materials(path string) ([]Material, error) {
	blob, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(blob) < 12 || binary.LittleEndian.Uint32(blob) != magic {
		return nil, fmt.Errorf("%s: not a glTF binary", path)
	}

	var doc struct {
		Materials []struct {
			Name string `json:"name"`
			PBR  struct {
				BaseColorFactor []float32 `json:"baseColorFactor"`
			} `json:"pbrMetallicRoughness"`
		} `json:"materials"`
	}

	for off := 12; off+8 <= len(blob); {
		length := int(binary.LittleEndian.Uint32(blob[off:]))
		kind := binary.LittleEndian.Uint32(blob[off+4:])
		body := off + 8
		if body+length > len(blob) {
			return nil, fmt.Errorf("%s: chunk runs past the end of the file", path)
		}
		if kind == chunkJSON {
			if err := json.Unmarshal(blob[body:body+length], &doc); err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			out := make([]Material, 0, len(doc.Materials))
			for _, m := range doc.Materials {
				// glTF omits baseColorFactor when it is white.
				colour := [3]float32{1, 1, 1}
				copy(colour[:], m.PBR.BaseColorFactor)
				out = append(out, Material{Name: m.Name, BaseColor: colour})
			}
			return out, nil
		}
		off = body + length
	}
	return nil, fmt.Errorf("%s: no JSON chunk", path)
}
