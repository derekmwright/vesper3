import bpy
import math
import os
import sys
from mathutils import Vector

# Run: blender --background --python script.py -- out.glb
if "--" not in sys.argv:
    raise RuntimeError("Expected output path after literal --.")
args = sys.argv[sys.argv.index("--") + 1:]
if not args or not args[-1].strip():
    raise RuntimeError("Missing output path after --.")
output_path = os.path.abspath(args[-1])
if not output_path.lower().endswith(".glb"):
    raise RuntimeError("Output path must end in .glb.")

# Remove all default objects and worlds.
for obj in list(bpy.data.objects):
    bpy.data.objects.remove(obj, do_unlink=True)
for scene in bpy.data.scenes:
    scene.world = None
for world in list(bpy.data.worlds):
    bpy.data.worlds.remove(world, do_unlink=True)

scene = bpy.context.scene
scene.unit_settings.system = "METRIC"
scene.unit_settings.scale_length = 1.0
scene.unit_settings.length_unit = "METERS"


def material(name, color, metallic, roughness, emission=0.0):
    mat = bpy.data.materials.new(name)
    mat.diffuse_color = (*color, 1.0)
    mat.use_nodes = True
    nodes = mat.node_tree.nodes
    nodes.clear()
    shader = nodes.new("ShaderNodeBsdfPrincipled")
    shader.inputs["Base Color"].default_value = (*color, 1.0)
    shader.inputs["Metallic"].default_value = metallic
    shader.inputs["Roughness"].default_value = roughness
    if emission:
        shader.inputs["Emission Color"].default_value = (*color, 1.0)
        shader.inputs["Emission Strength"].default_value = emission
    output = nodes.new("ShaderNodeOutputMaterial")
    mat.node_tree.links.new(shader.outputs["BSDF"], output.inputs["Surface"])
    return mat


metal = material("Cool blue-grey metal", (0.22, 0.32, 0.40), 0.70, 0.38)
amber = material("Amber charge indicator", (1.0, 0.32, 0.035), 0.0, 0.30, 2.5)

# Assemble every component directly into one mesh: no UVs or modifiers.
vertices = []
faces = []
material_indices = []


def append_geometry(points, polygons, material_index=0):
    offset = len(vertices)
    vertices.extend(points)
    faces.extend(tuple(offset + i for i in polygon) for polygon in polygons)
    material_indices.extend([material_index] * len(polygons))


def box(cx, cy, z0, z1, width, depth, material_index=0):
    x0, x1 = cx - width / 2, cx + width / 2
    y0, y1 = cy - depth / 2, cy + depth / 2
    append_geometry(
        [
            (x0, y0, z0), (x1, y0, z0),
            (x1, y1, z0), (x0, y1, z0),
            (x0, y0, z1), (x1, y0, z1),
            (x1, y1, z1), (x0, y1, z1),
        ],
        [
            (3, 2, 1, 0), (4, 5, 6, 7),
            (0, 1, 5, 4), (1, 2, 6, 5),
            (2, 3, 7, 6), (3, 0, 4, 7),
        ],
        material_index,
    )


def ring_solid(cx, cy, rings, sides, phase=0.0):
    """rings contains (z, x_radius, y_radius), ordered bottom to top."""
    points = []
    for z, rx, ry in rings:
        for i in range(sides):
            angle = phase + 2.0 * math.pi * i / sides
            points.append((cx + rx * math.cos(angle),
                           cy + ry * math.sin(angle), z))

    polygons = [tuple(reversed(range(sides)))]
    for level in range(len(rings) - 1):
        lower = level * sides
        upper = lower + sides
        for i in range(sides):
            j = (i + 1) % sides
            polygons.append((lower + i, lower + j, upper + j, upper + i))
    top = (len(rings) - 1) * sides
    polygons.append(tuple(top + i for i in range(sides)))
    append_geometry(points, polygons)


# Low octagonal pad with a narrow bevel around its upper edge.
ring_solid(
    0.0, 0.0,
    [(0.0, 0.400, 0.305),
     (0.045, 0.400, 0.305),
     (0.060, 0.382, 0.287)],
    sides=8,
    phase=math.pi / 8,
)

# Frame: lower cradle, outer uprights, upper bridge, and rear braces.
box(0.0, 0.0, 0.055, 0.100, 0.700, 0.270)
for x in (-0.329, 0.329):
    box(x, 0.0, 0.080, 0.750, 0.038, 0.220)
box(0.0, 0.0, 0.716, 0.750, 0.696, 0.220)
box(0.0, 0.121, 0.185, 0.222, 0.640, 0.027)
box(0.0, 0.121, 0.565, 0.602, 0.640, 0.027)

# Three upright cells. Faceted shoulder rings round their silhouettes.
# Twelve sides provide a broad, flat forward-facing surface at negative Y.
phase = math.pi / 12
for x in (-0.205, 0.0, 0.205):
    ring_solid(
        x, 0.0,
        [(0.095, 0.071, 0.094),
         (0.108, 0.084, 0.108),
         (0.130, 0.091, 0.116),
         (0.640, 0.091, 0.116),
         (0.662, 0.084, 0.108),
         (0.676, 0.068, 0.090)],
        sides=12,
        phase=phase,
    )
    # Metal terminal connects each cell to the upper frame.
    ring_solid(
        x, 0.0,
        [(0.670, 0.038, 0.045),
         (0.713, 0.038, 0.045),
         (0.722, 0.031, 0.038)],
        sides=8,
        phase=math.pi / 8,
    )

# Raised metal indicator surround on the middle cell's front.
box(0.0, -0.115, 0.269, 0.571, 0.054, 0.020)
# Exactly one warm accent detail: a single uninterrupted glowing strip.
box(0.0, -0.126, 0.283, 0.557, 0.026, 0.008, material_index=1)

mesh = bpy.data.meshes.new("EnergyStorageBankMesh")
mesh.from_pydata(vertices, [], faces)
mesh.materials.append(metal)
mesh.materials.append(amber)
mesh.update()

for polygon, material_index in zip(mesh.polygons, material_indices):
    polygon.material_index = material_index
    polygon.use_smooth = False

model = bpy.data.objects.new("EnergyStorageBank", mesh)
scene.collection.objects.link(model)
bpy.context.view_layer.objects.active = model
model.select_set(True)

# Explicitly preserve a ground contact plane at exactly Z = 0.
min_local_z = min(vertex.co.z for vertex in mesh.vertices)
for vertex in mesh.vertices:
    vertex.co.z -= min_local_z
mesh.update()
bpy.context.view_layer.update()

# Validate final world-space dimensions and actual triangulated face count.
points = [model.matrix_world @ vertex.co for vertex in mesh.vertices]
if not points or any(not math.isfinite(v) for point in points for v in point):
    raise RuntimeError("Mesh has missing or non-finite coordinates.")

radius = max(math.hypot(point.x, point.y) for point in points)
min_z = min(point.z for point in points)
max_z = max(point.z for point in points)
height = max_z - min_z
mesh.calc_loop_triangles()
triangles = len(mesh.loop_triangles)

if radius > 0.80:
    raise RuntimeError(f"Footprint radius exceeds 0.80 m: {radius:.9f}")
if min_z != 0.0:
    raise RuntimeError(f"Minimum Z must be exactly zero: {min_z:.12f}")
if abs(height - 0.750) > 0.00001:
    raise RuntimeError(f"Height must be 0.750 m: {height:.9f}")
if triangles >= 1500:
    raise RuntimeError(f"Triangle count must be under 1500: {triangles}")
if len(scene.objects) != 1 or model.type != "MESH":
    raise RuntimeError("Scene must contain exactly one mesh object.")
if mesh.uv_layers or model.modifiers:
    raise RuntimeError("Mesh must have no UVs or modifiers.")
if any(polygon.use_smooth for polygon in mesh.polygons):
    raise RuntimeError("All polygons must be flat shaded.")
if scene.world is not None:
    raise RuntimeError("Scene must have no world.")
if sum(p.material_index == 1 for p in mesh.polygons) != 6:
    raise RuntimeError("Expected exactly one box-shaped amber indicator.")

os.makedirs(os.path.dirname(output_path), exist_ok=True)
print(f"EnergyStorageBank: {triangles} triangles, height {height:.3f} m", flush=True)

result = bpy.ops.export_scene.gltf(
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
if "FINISHED" not in result:
    raise RuntimeError(f"glTF export failed: {result}")
