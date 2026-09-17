import bpy
import bmesh
import math
import os
import sys
from mathutils import Vector

# Usage: blender --background --python script.py -- out.glb
if "--" not in sys.argv:
    raise RuntimeError("Expected output path after a literal --")
arguments = sys.argv[sys.argv.index("--") + 1:]
if not arguments or not arguments[-1].strip():
    raise RuntimeError("Missing output GLB path")
output_path = os.path.abspath(arguments[-1])
if not output_path.lower().endswith(".glb"):
    raise RuntimeError("Output filename must end in .glb")
if not os.path.isdir(os.path.dirname(output_path)):
    raise RuntimeError("Output directory does not exist")

# Clear all default objects, including objects outside the active collection.
for obj in list(bpy.data.objects):
    bpy.data.objects.remove(obj, do_unlink=True)
for world in list(bpy.data.worlds):
    bpy.data.worlds.remove(world, do_unlink=True)

scene = bpy.context.scene
scene.world = None
scene.unit_settings.system = "METRIC"
scene.unit_settings.scale_length = 1.0
scene.unit_settings.length_unit = "METERS"

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

shell_mat = material("Pale off-white shell", (0.82, 0.84, 0.80), 0.12, 0.72)
metal_mat = material("Cool blue-grey metal", (0.25, 0.34, 0.41), 0.65, 0.52)
amber_mat = material("Warm amber marker", (0.95, 0.39, 0.055), 0.15, 0.48)
parts = []

def register(obj, name, mat):
    obj.name = name
    obj.data.materials.clear()
    obj.data.materials.append(mat)
    for polygon in obj.data.polygons:
        polygon.use_smooth = False
    parts.append(obj)
    return obj

def box(name, center, dimensions, mat):
    bpy.ops.mesh.primitive_cube_add(size=1.0, calc_uvs=False, location=center)
    obj = bpy.context.object
    obj.scale = dimensions
    bpy.ops.object.transform_apply(location=False, rotation=False, scale=True)
    return register(obj, name, mat)

# Low octagonal foundation: bottom 0.00 m, top 0.12 m.
bpy.ops.mesh.primitive_cylinder_add(
    vertices=8,
    radius=0.75,
    depth=0.12,
    end_fill_type="NGON",
    calc_uvs=False,
    location=(0.0, 0.0, 0.06),
    rotation=(0.0, 0.0, math.pi / 8.0),
)
register(bpy.context.object, "Octagonal foundation", metal_mat)

# A true subdivision-2 icosphere, bisected at its equator.
# Slight vertical stretching gives a compact, tall pressure shell.
bpy.ops.mesh.primitive_ico_sphere_add(
    subdivisions=2, radius=1.0, calc_uvs=False, location=(0.0, 0.0, 0.0)
)
dome = bpy.context.object
bm = bmesh.new()
bm.from_mesh(dome.data)
bmesh.ops.bisect_plane(
    bm,
    geom=list(bm.verts) + list(bm.edges) + list(bm.faces),
    dist=1e-7,
    plane_co=(0.0, 0.0, 0.0),
    plane_no=(0.0, 0.0, 1.0),
    clear_inner=True,
    clear_outer=False,
)
if not bm.verts or min(v.co.z for v in bm.verts) < -1e-6:
    raise RuntimeError("Icosphere hemisphere cut failed")

boundary = [edge for edge in bm.edges if edge.is_boundary]
if not boundary:
    raise RuntimeError("Hemisphere has no equatorial boundary")
bmesh.ops.holes_fill(bm, edges=boundary, sides=0)
for vertex in bm.verts:
    vertex.co.x *= 0.64
    vertex.co.y *= 0.64
    vertex.co.z = max(0.0, vertex.co.z) * 0.76 + 0.12
bmesh.ops.recalc_face_normals(bm, faces=list(bm.faces))
bm.to_mesh(dome.data)
bm.free()
dome.data.update()
register(dome, "Geodesic pressure shell", shell_mat)

# The airlock overlaps the shell and projects toward +X.
box(
    "Airlock chamber",
    (0.615, 0.0, 0.29),
    (0.29, 0.32, 0.34),
    metal_mat,
)

# Raised, clipped-corner hatch on the airlock's outer end.
outline = [
    (-0.085, 0.145),
    (0.085, 0.145),
    (0.115, 0.175),
    (0.115, 0.365),
    (0.085, 0.395),
    (-0.085, 0.395),
    (-0.115, 0.365),
    (-0.115, 0.175),
]
vertices = [(x, y, z) for x in (0.757, 0.771) for y, z in outline]
faces = [
    tuple(reversed(range(8))),
    tuple(range(8, 16)),
]
for i in range(8):
    j = (i + 1) % 8
    faces.append((i, j, j + 8, i + 8))
mesh = bpy.data.meshes.new("Clipped hatch mesh")
mesh.from_pydata(vertices, [], faces)
mesh.update()
hatch = bpy.data.objects.new("Airlock hatch", mesh)
scene.collection.objects.link(hatch)
register(hatch, "Airlock hatch", metal_mat)

# One non-emissive amber accent above the hatch.
box(
    "Amber identification strip",
    (0.765, 0.0, 0.427),
    (0.012, 0.18, 0.027),
    amber_mat,
)

# Join every component into one mesh with the origin at the pad center.
bpy.ops.object.select_all(action="DESELECT")
for obj in parts:
    obj.select_set(True)
bpy.context.view_layer.objects.active = dome
bpy.ops.object.join()
habitat = bpy.context.object
habitat.name = "Habitat"
habitat.data.name = "HabitatMesh"
bpy.ops.object.transform_apply(location=True, rotation=True, scale=True)

# Bake triangulation and flat normals; no modifiers or UV maps.
bm = bmesh.new()
bm.from_mesh(habitat.data)
bmesh.ops.triangulate(bm, faces=list(bm.faces))
bmesh.ops.recalc_face_normals(bm, faces=list(bm.faces))
bm.to_mesh(habitat.data)
bm.free()
for uv_layer in list(habitat.data.uv_layers):
    habitat.data.uv_layers.remove(uv_layer)
for polygon in habitat.data.polygons:
    polygon.use_smooth = False

# Set the lowest vertex to exactly zero, eliminating roundoff.
minimum_z = min(vertex.co.z for vertex in habitat.data.vertices)
for vertex in habitat.data.vertices:
    vertex.co.z -= minimum_z
    if abs(vertex.co.z) < 1e-7:
        vertex.co.z = 0.0
habitat.data.update()
bpy.context.view_layer.update()

def require(condition, message):
    if not condition:
        raise RuntimeError(message)

coords = [habitat.matrix_world @ v.co for v in habitat.data.vertices]
require(all(math.isfinite(c) for v in coords for c in v),
        "Mesh contains non-finite coordinates")
require(min(v.z for v in coords) == 0.0, "Minimum Z must be exactly zero")
require(all(v.z >= 0.0 for v in coords), "Geometry extends below ground")
require(max(math.hypot(v.x, v.y) for v in coords) <= 0.80,
        "Horizontal footprint exceeds 0.80 m radius")
require(0.85 <= max(v.z for v in coords) <= 0.95,
        "Total height must be approximately 0.90 m")

habitat.data.calc_loop_triangles()
triangle_count = len(habitat.data.loop_triangles)
require(0 < triangle_count < 1500, "Triangle budget exceeded")
require(len(scene.objects) == 1 and scene.objects[0] == habitat,
        "Scene must contain exactly one object")
require(habitat.type == "MESH" and habitat.name == "Habitat",
        "Final object must be a mesh named Habitat")
require(len(habitat.modifiers) == 0, "Modifiers are forbidden")
require(len(habitat.data.uv_layers) == 0, "UV maps are forbidden")
require(all(not p.use_smooth for p in habitat.data.polygons),
        "All faces must use flat shading")
require(scene.world is None and len(bpy.data.worlds) == 0,
        "World datablocks are forbidden")
require(len(habitat.data.materials) == 3, "Expected exactly three materials")
for mat in habitat.data.materials:
    require(all(node.type in {"BSDF_PRINCIPLED", "OUTPUT_MATERIAL"}
                for node in mat.node_tree.nodes),
            "Materials must contain only untextured Principled shaders")

result = bpy.ops.export_scene.gltf(
    filepath=output_path,
    check_existing=False,
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
require("FINISHED" in result, "glTF export failed")
require(os.path.isfile(output_path) and os.path.getsize(output_path) > 20,
        "Export did not produce a valid-sized file")
with open(output_path, "rb") as exported:
    require(exported.read(4) == b"glTF", "Output is not a GLB file")
print(f"Exported Habitat: {triangle_count} triangles, "
      f"{max(v.z for v in coords):.3f} m tall -> {output_path}")
