package meshgen

import (
	"testing"

	"github.com/derekmwright/vesper3/internal/world"
)

// stepped builds the origin chunk of a map whose rows step down away from the
// top edge, with the first two rows and the rest given whatever terrain the
// caller asks for.
//
// Every variant below has identical elevations and differs only in terrain, so
// nothing about the map rim or the step heights can move between them. The
// only thing that can change the vertex count is which edges get a wall.
func stepped(t *testing.T, shallow, deep world.Terrain) int {
	t.Helper()
	m, err := world.NewMap(8, 8, 1)
	if err != nil {
		t.Fatal(err)
	}
	for row := range 8 {
		for col := range 8 {
			tile := m.AtOffset(col, row)
			if row < 2 {
				tile.Terrain, tile.Elevation = shallow, world.SeaLevel+3
				continue
			}
			tile.Terrain, tile.Elevation = deep, world.SeaLevel-int8(row)
		}
	}
	id, ok := ChunkOf(m, world.FromOffset(0, 0))
	if !ok {
		t.Fatal("no chunk at the origin")
	}
	verts, _ := BuildChunk(m, id, nil, nil)
	if len(verts) == 0 {
		t.Fatal("the chunk built nothing")
	}
	return len(verts)
}

// The sea floor has the same varied elevation the land does, and every drop
// used to get a cliff face - shaded darker than the caps around it, and
// visible through the water as dark patches scattered over the shelf. Nothing
// can be reached or built down there, so those faces existed only to be seen
// as noise. On seed 7 they were 3,160 vertices, 7.4% of the terrain mesh.
func TestTheSeaFloorIsNotWalledButTheCoastIs(t *testing.T) {
	// The same stepped shelf, once as dry land and once as sea bed.
	dry := stepped(t, world.Regolith, world.Regolith)
	wet := stepped(t, world.Regolith, world.Sea)

	if wet >= dry {
		t.Errorf("a submerged shelf builds %d vertices and a dry one %d: "+
			"the steps between submerged tiles are still being walled", wet, dry)
	}

	// And the coastline keeps its cliff. Flooding the high rows too turns the
	// land-to-sea drop into another sea-to-sea one, which is skipped - so if
	// that is smaller again, the wall in `wet` was the coast.
	drowned := stepped(t, world.Sea, world.Sea)
	if drowned >= wet {
		t.Errorf("open water builds %d vertices against a coastline's %d: "+
			"the land-to-sea cliff is not being drawn", drowned, wet)
	}
}
