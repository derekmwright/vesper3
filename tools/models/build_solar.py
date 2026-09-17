import bpy
import math
import os
import sys
from mathutils import Vector

# Run: blender --background --python script.py -- out.glb
if "--" not in sys.argv:
    raise RuntimeError("Expected an output path after --.")
arguments = sys.argv[sys.argv.index("--") + 1:]
if not arguments or not arguments[-1].strip():
    raise RuntimeError("Missing output GLB path.")
output_path = os.path.abspath(arguments[-1])

# Clear all scene objects and worlds.
for obj in list(bpy.data.objects):
    bpy.data.objects.remove(obj, do_unlink=True)
for scene in bpy.data.scenes:
    scene.world = None
for world in list(bpy.data.worlds):
    bpy.data.worlds.remove(world, do_unlink=True)

scene = bpy.context.scene
scene.unit_settings.system = "METRIC"
scene.unit_settings.length_unit = "METERS"
scene.unit_settings.scale_length = 1.0


def material(name, color, metallic, roughness):
    mat = bpy.data.materials.new(name)
    mat.use_nodes = True
    mat.diffuse_color = (*color, 1.0)
    nodes = mat.node_tree.nodes
    nodes.clear()
    shader = nodes.new("ShaderNodeBsdfPrincipled")
    shader.inputs["Base Color"].default_value = (*color, 1.0)
    shader.inputs["Metallic"].default_value = metallic
    shader.inputs["Roughness"].default_value = roughness
    output = nodes.new("ShaderNodeOutputMaterial")
    mat.node_tree.links.new(shader.outputs["BSDF"], output.inputs["Surface"])
    return mat


materials = [
    material("Cool blue-grey metal", (0.24, 0.33, 0.41), 0.65, 0.48),
    material("Dark blue photovoltaic cells", (0.012, 0.040, 0.115), 0.25, 0.32),
    material("Amber identification tab", (0.95, 0.36, 0.055), 0.15, 0.48),
]

vertices = []
faces = []
material_indices = []


def add_geometry(points, polygons, material_index):
    offset = len(vertices)
    vertices.extend(points)
    faces.extend(tuple(offset + index for index in face) for face in polygons)
    material_indices.extend([material_index] * len(polygons))


def add_box(center, dimensions, material_index=0, tilt=0.0):
    hx, hy, hz = (value * 0.5 for value in dimensions)
    local_points = [
        (-hx, -hy, -hz), (hx, -hy, -hz),
        (hx, hy, -hz), (-hx, hy, -hz),
        (-hx, -hy, hz), (hx, -hy, hz),
        (hx, hy, hz), (-hx, hy, hz),
    ]
    cosine, sine = math.cos(tilt), math.sin(tilt)
    cx, cy, cz = center
    points = [
        (cx + x, cy + y * cosine - z * sine,
         cz + y * sine + z * cosine)
        for x, y, z in local_points
    ]
    polygons = [
        (0, 3, 2, 1), (4, 5, 6, 7),
        (0, 1, 5, 4), (1, 2, 6, 5),
        (2, 3, 7, 6), (3, 0, 4, 7),
    ]
    add_geometry(points, polygons, material_index)


# Low octagonal pad, with bottom vertices exactly on Z = 0.
pad_radius = 0.74
pad_height = 0.07
pad_points = [
    (pad_radius * math.cos(math.pi / 8 + i * math.tau / 8),
     pad_radius * math.sin(math.pi / 8 + i * math.tau / 8),
     z)
    for z in (0.0, pad_height)
    for i in range(8)
]
pad_faces = [
    tuple(reversed(range(8))),
    tuple(range(8, 16)),
]
pad_faces.extend(
    (i, (i + 1) % 8, (i + 1) % 8 + 8, i + 8)
    for i in range(8)
)
add_geometry(pad_points, pad_faces, 0)

# Three identical framed panels, all tilted toward the same direction.
tilt = math.radians(25.0)
sine, cosine = math.sin(tilt), math.cos(tilt)
panel_width = 0.36
panel_length = 0.66
frame_thickness = 0.035
target_height = 0.55
panel_center_z = target_height - (
    panel_length * 0.5 * sine + frame_thickness * 0.5 * cosine
)


def panel_point(x_center, local_x, local_y, local_z):
    return (
        x_center + local_x,
        local_y * cosine - local_z * sine,
        panel_center_z + local_y * sine + local_z * cosine,
    )


for x_center in (-0.43, 0.0, 0.43):
    # Foot and short upright support. The post intersects the frame.
    add_box((x_center, 0.0, 0.085), (0.13, 0.14, 0.03))
    post_bottom = pad_height
    post_top = panel_center_z
    add_box(
        (x_center, 0.0, (post_bottom + post_top) * 0.5),
        (0.065, 0.075, post_top - post_bottom),
    )

    # Metal backing doubles as the visible border around the cells.
    add_box(
        (x_center, 0.0, panel_center_z),
        (panel_width, panel_length, frame_thickness),
        0, tilt,
    )

    # Six dark-blue cell blocks form one panel face, separated by thin
    # exposed metal seams. Geometry supplies detail without UVs or textures.
    active_width = 0.32
    active_length = 0.60
    gap = 0.006
    cell_width = (active_width - gap) / 2
    cell_length = (active_length - 2 * gap) / 3
    cell_thickness = 0.008
    cell_center_z = frame_thickness * 0.5 + cell_thickness * 0.5 - 0.001

    for column in range(2):
        local_x = -active_width * 0.5 + cell_width * 0.5
        local_x += column * (cell_width + gap)
        for row in range(3):
            local_y = -active_length * 0.5 + cell_length * 0.5
            local_y += row * (cell_length + gap)
            add_box(
                panel_point(x_center, local_x, local_y, cell_center_z),
                (cell_width, cell_length, cell_thickness),
                1, tilt,
            )

# Exactly one warm amber detail: a raised identification tab on the pad.
add_box((0.0, -0.57, 0.082), (0.11, 0.055, 0.028), 2)

# Assemble every component directly into one mesh object.
mesh = bpy.data.meshes.new("SolarArrayMesh")
mesh.from_pydata(vertices, [], faces)
mesh.update()
for mat in materials:
    mesh.materials.append(mat)
for polygon, material_index in zip(mesh.polygons, material_indices):
    polygon.material_index = material_index
    polygon.use_smooth = False

obj = bpy.data.objects.new("SolarArray", mesh)
scene.collection.objects.link(obj)
bpy.context.view_layer.objects.active = obj
obj.select_set(True)
bpy.context.view_layer.update()

# Validate the actual mesh in world-space metres.
points = [obj.matrix_world @ vertex.co for vertex in mesh.vertices]
if not points or any(not math.isfinite(v) for point in points for v in point):
    raise RuntimeError("Mesh contains missing or invalid coordinates.")

radius = max(math.hypot(point.x, point.y) for point in points)
minimum_z = min(point.z for point in points)
maximum_z = max(point.z for point in points)
height = maximum_z - minimum_z

mesh.calc_loop_triangles()
triangle_count = len(mesh.loop_triangles)

if radius > 0.80:
    raise RuntimeError(f"Footprint radius exceeds 0.80 m: {radius:.9f}")
if minimum_z != 0.0:
    raise RuntimeError(f"Minimum Z must be exactly zero: {minimum_z:.9f}")
if abs(height - target_height) > 0.000001:
    raise RuntimeError(f"Height must be approximately 0.55 m: {height:.9f}")
if triangle_count >= 1500:
    raise RuntimeError(f"Triangle count must be under 1500: {triangle_count}")
if len(scene.objects) != 1 or obj.type != "MESH":
    raise RuntimeError("Scene must contain exactly one mesh object.")
if mesh.uv_layers or obj.modifiers:
    raise RuntimeError("UV layers and modifiers are not permitted.")
if any(polygon.use_smooth for polygon in mesh.polygons):
    raise RuntimeError("Every face must use flat shading.")
if scene.world is not None:
    raise RuntimeError("Scene must have no world.")

os.makedirs(os.path.dirname(output_path), exist_ok=True)
print(f"SolarArray: {triangle_count} triangles, height {height:.6f} m", flush=True)

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
)
if "FINISHED" not in result:
    raise RuntimeError(f"GLB export failed: {result}")
