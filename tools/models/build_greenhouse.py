import sys
import math
import bpy
import bmesh
from mathutils import Vector

# Usage: blender --background --python script.py -- out.glb
if "--" not in sys.argv:
    raise RuntimeError("Expected an output path after '--'.")
arguments = sys.argv[sys.argv.index("--") + 1:]
if not arguments or not arguments[-1].lower().endswith(".glb"):
    raise RuntimeError("The last argument after '--' must be a .glb path.")
output_path = bpy.path.abspath(arguments[-1])

# Clear the entire scene, including objects outside the active collection.
for obj in list(bpy.data.objects):
    bpy.data.objects.remove(obj, do_unlink=True)
for scene in bpy.data.scenes:
    scene.world = None
for world in list(bpy.data.worlds):
    bpy.data.worlds.remove(world)

scene = bpy.context.scene
scene.unit_settings.system = "METRIC"
scene.unit_settings.scale_length = 1.0
scene.unit_settings.length_unit = "METERS"

def material(name, color, metallic, roughness):
    mat = bpy.data.materials.new(name)
    mat.use_nodes = True
    mat.diffuse_color = (*color, 1.0)
    bsdf = mat.node_tree.nodes.get("Principled BSDF")
    bsdf.inputs["Base Color"].default_value = (*color, 1.0)
    bsdf.inputs["Metallic"].default_value = metallic
    bsdf.inputs["Roughness"].default_value = roughness
    return mat

metal = material("Cool blue-grey metal", (0.30, 0.40, 0.46), 0.65, 0.48)
# Pale opaque color suggests frosted glass without transmission or textures.
glass = material("Pale frosted green panels", (0.57, 0.79, 0.65), 0.0, 0.30)
amber = material("Single warm amber plaque", (0.95, 0.43, 0.075), 0.25, 0.40)

parts = []

def mesh_part(name, vertices, faces, mat):
    mesh = bpy.data.meshes.new(name + "_Mesh")
    mesh.from_pydata(vertices, [], faces)
    mesh.update()
    obj = bpy.data.objects.new(name, mesh)
    scene.collection.objects.link(obj)
    obj.data.materials.append(mat)
    parts.append(obj)
    return obj

BOX_FACES = [
    (0, 3, 2, 1),
    (4, 5, 6, 7),
    (0, 1, 5, 4),
    (1, 2, 6, 5),
    (2, 3, 7, 6),
    (3, 0, 4, 7),
]

def box(name, center, dimensions, mat=metal):
    cx, cy, cz = center
    hx, hy, hz = (d * 0.5 for d in dimensions)
    vertices = [
        (cx-hx, cy-hy, cz-hz), (cx+hx, cy-hy, cz-hz),
        (cx+hx, cy+hy, cz-hz), (cx-hx, cy+hy, cz-hz),
        (cx-hx, cy-hy, cz+hz), (cx+hx, cy-hy, cz+hz),
        (cx+hx, cy+hy, cz+hz), (cx-hx, cy+hy, cz+hz),
    ]
    return mesh_part(name, vertices, BOX_FACES, mat)

def prism_y(name, profile_xz, y_min, y_max, mat):
    count = len(profile_xz)
    vertices = [(x, y_min, z) for x, z in profile_xz]
    vertices += [(x, y_max, z) for x, z in profile_xz]
    faces = [
        tuple(reversed(range(count))),
        tuple(range(count, count * 2)),
    ]
    for i in range(count):
        j = (i + 1) % count
        faces.append((i, j, j + count, i + count))
    return mesh_part(name, vertices, faces, mat)

def beam(name, start, end, width, depth, mat=metal):
    a, b = Vector(start), Vector(end)
    axis = (b - a).normalized()
    reference = Vector((0, 0, 1))
    if abs(axis.dot(reference)) > 0.95:
        reference = Vector((0, 1, 0))
    u = axis.cross(reference).normalized() * (width * 0.5)
    v = axis.cross(u).normalized() * (depth * 0.5)
    vertices = []
    for point in (a, b):
        vertices.extend([
            tuple(point-u-v), tuple(point+u-v),
            tuple(point+u+v), tuple(point-u+v),
        ])
    return mesh_part(name, vertices, BOX_FACES, mat)

# Ground pad: corner radius sqrt(0.40^2 + 0.68^2) = 0.7889 m.
# Its bottom vertices are exactly z=0.
box("Low rectangular base pad", (0, 0, 0.0325), (0.80, 1.36, 0.065))
box("Raised perimeter sill", (0, 0, 0.080), (0.72, 1.25, 0.030))

# Solid pentagonal metal end walls.
end_profile = [
    (-0.35, 0.065),
    (0.35, 0.065),
    (0.35, 0.464),
    (0.0, 0.674),
    (-0.35, 0.464),
]
prism_y("Front solid gable", end_profile, -0.625, -0.603, metal)
prism_y("Rear solid gable", end_profile, 0.603, 0.625, metal)

# Four glass bays along each long side, separated by structural posts.
bay_edges = [-0.603, -0.3015, 0.0, 0.3015, 0.603]
for side in (-1, 1):
    x = side * 0.343
    for i in range(4):
        lo, hi = bay_edges[i], bay_edges[i + 1]
        box(
            "Side glass %d %d" % (side, i),
            (x, (lo + hi) * 0.5, 0.277),
            (0.010, hi - lo - 0.019, 0.354),
            glass,
        )
    for i, y in enumerate(bay_edges):
        box(
            "Side post %d %d" % (side, i),
            (side * 0.350, y, 0.279),
            (0.024, 0.024, 0.382),
        )
    box("Eave rail %d" % side,
        (side * 0.350, 0, 0.459), (0.032, 1.28, 0.026))
    box("Side sill rail %d" % side,
        (side * 0.350, 0, 0.103), (0.028, 1.26, 0.023))

# Two continuous thin roof slabs meet precisely at the ridge.
for side in (-1, 1):
    roof_profile = [
        (0.0, 0.674),
        (side * 0.374, 0.455),
        (side * 0.374, 0.465),
        (0.0, 0.684),
    ]
    prism_y("Angled green roof %d" % side,
            roof_profile, -0.647, 0.647, glass)

# Slim cross rafters divide the roof into readable low-poly panels.
for i, y in enumerate((-0.635, -0.3015, 0.0, 0.3015, 0.635)):
    for side in (-1, 1):
        beam(
            "Roof rafter %d %d" % (side, i),
            (0, y, 0.685),
            (side * 0.374, y, 0.466),
            0.016, 0.010,
        )

# The ridge cap establishes the overall height at 0.700 m.
box("Metal ridge cap", (0, 0, 0.690), (0.026, 1.31, 0.020))

# A simple raised door outline on the front solid end.
for x in (-0.105, 0.105):
    box("Door jamb", (x, -0.636, 0.242), (0.012, 0.014, 0.306))
box("Door lintel", (0, -0.636, 0.395), (0.222, 0.014, 0.014))

# Exactly one warm amber detail in the entire model.
box("Single amber door plaque",
    (0.063, -0.650, 0.265), (0.031, 0.012, 0.057), amber)

# Join all geometry into one mesh object.
bpy.ops.object.select_all(action="DESELECT")
for obj in parts:
    obj.select_set(True)
bpy.context.view_layer.objects.active = parts[0]
bpy.ops.object.join()
model = bpy.context.view_layer.objects.active
model.name = "LowPoly_Greenhouse"
model.data.name = "LowPoly_Greenhouse_Mesh"

# Recalculate outward normals and explicitly triangulate for predictable export.
bm = bmesh.new()
bm.from_mesh(model.data)
bmesh.ops.recalc_face_normals(bm, faces=list(bm.faces))
bmesh.ops.triangulate(bm, faces=list(bm.faces))
bm.to_mesh(model.data)
bm.free()

for polygon in model.data.polygons:
    polygon.use_smooth = False
for uv_layer in list(model.data.uv_layers):
    model.data.uv_layers.remove(uv_layer)
model.data.update()
bpy.context.view_layer.update()

# Validate the final geometry in world coordinates.
points = [model.matrix_world @ vertex.co for vertex in model.data.vertices]
if not points or any(not math.isfinite(c) for p in points for c in p):
    raise RuntimeError("Model geometry is empty or contains non-finite values.")

radius = max(math.hypot(p.x, p.y) for p in points)
min_z = min(p.z for p in points)
max_z = max(p.z for p in points)
height = max_z - min_z
model.data.calc_loop_triangles()
triangles = len(model.data.loop_triangles)

if radius > 0.80:
    raise RuntimeError("Footprint radius exceeds 0.80 m: %.9f" % radius)
if min_z != 0.0:
    raise RuntimeError("Minimum Z must be exactly 0.0: %.12f" % min_z)
if abs(height - 0.70) > 1e-6:
    raise RuntimeError("Total height must be 0.70 m: %.9f" % height)
if triangles >= 1500:
    raise RuntimeError("Triangle count must be under 1500: %d" % triangles)
if len(scene.objects) != 1 or model.type != "MESH":
    raise RuntimeError("Scene must contain exactly one mesh object.")
if model.modifiers or model.data.uv_layers:
    raise RuntimeError("Unexpected modifiers or UV layers.")
if scene.world is not None:
    raise RuntimeError("Scene must not contain a world.")
if sum(p.material_index == 2 for p in model.data.polygons) != 12:
    raise RuntimeError("Expected exactly one twelve-triangle amber plaque.")

print("Greenhouse: %d triangles, height %.3f m" % (triangles, height), flush=True)

bpy.ops.export_scene.gltf(
    filepath=output_path,
    export_format="GLB",
    use_selection=False,
    export_yup=True,
    export_apply=True,
    export_cameras=False,
    export_lights=False,
    export_texcoords=False,
    export_normals=True,
    export_materials="EXPORT",
    export_animations=False,
)
