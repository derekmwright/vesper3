import bpy
import bmesh
import math
import os
import sys
from mathutils import Vector


TARGET_HEIGHT = 1.15
MAX_RADIUS = 0.80
TRIANGLE_LIMIT = 1500
parts = []


def output_path():
    if "--" not in sys.argv:
        raise RuntimeError("Usage: blender --background --python script.py -- out.glb")
    arguments = sys.argv[sys.argv.index("--") + 1:]
    if not arguments or not arguments[-1].strip():
        raise RuntimeError("Missing output path after --")
    path = os.path.abspath(bpy.path.abspath(arguments[-1]))
    if not path.lower().endswith(".glb"):
        raise RuntimeError("Output path must end in .glb")
    return path


def material(name, color, metallic, roughness):
    mat = bpy.data.materials.new(name)
    mat.use_nodes = True
    mat.diffuse_color = (*color, 1.0)
    shader = mat.node_tree.nodes.get("Principled BSDF")
    shader.inputs["Base Color"].default_value = (*color, 1.0)
    shader.inputs["Metallic"].default_value = metallic
    shader.inputs["Roughness"].default_value = roughness
    return mat


def mesh_part(name, vertices, faces, mat):
    mesh = bpy.data.meshes.new(name)
    mesh.from_pydata(vertices, [], faces)
    mesh.update()

    bm = bmesh.new()
    bm.from_mesh(mesh)
    bmesh.ops.recalc_face_normals(bm, faces=list(bm.faces))
    bm.to_mesh(mesh)
    bm.free()

    obj = bpy.data.objects.new(name, mesh)
    bpy.context.scene.collection.objects.link(obj)
    mesh.materials.append(mat)
    for polygon in mesh.polygons:
        polygon.use_smooth = False
    parts.append(obj)
    return obj


def box(name, low, high, mat):
    x0, y0, z0 = low
    x1, y1, z1 = high
    vertices = [
        (x0, y0, z0), (x1, y0, z0),
        (x1, y1, z0), (x0, y1, z0),
        (x0, y0, z1), (x1, y0, z1),
        (x1, y1, z1), (x0, y1, z1),
    ]
    faces = [
        (3, 2, 1, 0), (4, 5, 6, 7),
        (0, 1, 5, 4), (1, 2, 6, 5),
        (2, 3, 7, 6), (3, 0, 4, 7),
    ]
    return mesh_part(name, vertices, faces, mat)


def beam(name, start, end, width, mat):
    a, b = Vector(start), Vector(end)
    axis = b - a
    if axis.length < 1e-8:
        raise RuntimeError("Zero-length beam")
    axis.normalize()
    helper = Vector((0, 0, 1))
    if abs(axis.dot(helper)) > 0.95:
        helper = Vector((0, 1, 0))
    u = axis.cross(helper).normalized() * (width / 2)
    v = axis.cross(u).normalized() * (width / 2)
    vertices = [
        tuple(p + su * u + sv * v)
        for p in (a, b)
        for su, sv in ((-1, -1), (1, -1), (1, 1), (-1, 1))
    ]
    faces = [
        (3, 2, 1, 0), (4, 5, 6, 7),
        (0, 1, 5, 4), (1, 2, 6, 5),
        (2, 3, 7, 6), (3, 0, 4, 7),
    ]
    return mesh_part(name, vertices, faces, mat)


def cylinder(name, start, end, radius, mat, sides=8):
    a, b = Vector(start), Vector(end)
    axis = (b - a).normalized()
    helper = Vector((0, 0, 1))
    if abs(axis.dot(helper)) > 0.95:
        helper = Vector((0, 1, 0))
    u = axis.cross(helper).normalized()
    v = axis.cross(u).normalized()
    vertices = []
    for center in (a, b):
        for i in range(sides):
            angle = 2 * math.pi * i / sides
            vertices.append(tuple(
                center + radius * (math.cos(angle) * u + math.sin(angle) * v)
            ))
    faces = [
        tuple(reversed(range(sides))),
        tuple(range(sides, 2 * sides)),
    ]
    for i in range(sides):
        j = (i + 1) % sides
        faces.append((i, j, sides + j, sides + i))
    return mesh_part(name, vertices, faces, mat)


def octagonal_ring(name, outer_center, outer_radius,
                   inner_center, inner_radius, bottom, top, mat):
    vertices = []
    for center, radius, z in (
        (outer_center, outer_radius, bottom),
        (outer_center, outer_radius, top),
        (inner_center, inner_radius, bottom),
        (inner_center, inner_radius, top),
    ):
        for i in range(8):
            angle = math.pi / 8 + i * math.pi / 4
            vertices.append((
                center[0] + radius * math.cos(angle),
                center[1] + radius * math.sin(angle),
                z,
            ))
    faces = []
    for i in range(8):
        j = (i + 1) % 8
        faces.extend([
            (i, j, 8 + j, 8 + i),
            (16 + j, 16 + i, 24 + i, 24 + j),
            (8 + i, 8 + j, 24 + j, 24 + i),
            (j, i, 16 + i, 16 + j),
        ])
    return mesh_part(name, vertices, faces, mat)


def main():
    path = output_path()

    # Remove all objects, including objects outside the active collection.
    if bpy.context.object and bpy.context.object.mode != "OBJECT":
        bpy.ops.object.mode_set(mode="OBJECT")
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

    steel = material("Cool blue-grey steel", (0.23, 0.32, 0.39), 0.70, 0.58)
    pad_mat = material("Cool slate pad", (0.13, 0.17, 0.21), 0.05, 0.90)
    dark = material("Dark shaft and hardware", (0.018, 0.023, 0.028), 0.25, 0.82)
    amber = material("Single amber identification plate", (0.95, 0.39, 0.055), 0.25, 0.48)

    cx, cy = -0.20, 0.0

    # A real opening through the pad, with a recessed dark floor above ground.
    octagonal_ring(
        "Octagonal base pad", (0, 0), 0.76,
        (cx, cy), 0.14, 0.0, 0.065, pad_mat,
    )
    cylinder(
        "Recessed shaft floor", (cx, cy, 0.0), (cx, cy, 0.012),
        0.14, dark,
    )
    octagonal_ring(
        "Shaft collar", (cx, cy), 0.174,
        (cx, cy), 0.14, 0.065, 0.098, steel,
    )
    octagonal_ring(
        "Dark inner shaft lining", (cx, cy), 0.140,
        (cx, cy), 0.134, 0.012, 0.097, dark,
    )

    # Four tapered legs, with three levels of crossed lattice bracing.
    corners = [(-1, -1), (1, -1), (1, 1), (-1, 1)]
    z_bottom, z_top = 0.100, 1.105

    def tower_point(corner, z):
        t = (z - z_bottom) / (z_top - z_bottom)
        half_width = 0.245 * (1 - t) + 0.092 * t
        sx, sy = corners[corner]
        return (cx + sx * half_width, cy + sy * half_width, z)

    for i in range(4):
        foot = tower_point(i, z_bottom)
        box(
            "Leg footing",
            (foot[0] - 0.049, foot[1] - 0.049, 0.065),
            (foot[0] + 0.049, foot[1] + 0.049, 0.111),
            steel,
        )
        beam(
            "Tower leg", tower_point(i, z_bottom),
            tower_point(i, z_top), 0.036, steel,
        )

    levels = [0.145, 0.455, 0.775, 1.085]
    for z in levels:
        for i in range(4):
            beam(
                "Horizontal lattice rail",
                tower_point(i, z), tower_point((i + 1) % 4, z),
                0.022, steel,
            )
    for low, high in zip(levels[:-1], levels[1:]):
        for i in range(4):
            j = (i + 1) % 4
            beam(
                "Lattice diagonal A",
                tower_point(i, low), tower_point(j, high),
                0.015, steel,
            )
            beam(
                "Lattice diagonal B",
                tower_point(j, low), tower_point(i, high),
                0.015, steel,
            )

    box(
        "Derrick crown",
        (cx - 0.133, -0.119, 1.105),
        (cx + 0.133, 0.119, TARGET_HEIGHT),
        steel,
    )
    cylinder(
        "Hoist sheave", (cx, -0.037, 1.056), (cx, 0.037, 1.056),
        0.057, steel, sides=10,
    )
    cylinder(
        "Sheave axle", (cx, -0.052, 1.056), (cx, 0.052, 1.056),
        0.015, dark,
    )
    beam(
        "Vertical hoist cable",
        (cx - 0.050, 0.0, 1.052), (cx - 0.050, 0.0, 0.030),
        0.007, dark,
    )

    # Low equipment shed beside the derrick.
    box("Shed plinth", (0.190, -0.230, 0.065), (0.625, 0.290, 0.097), pad_mat)
    box("Equipment shed", (0.215, -0.205, 0.097), (0.600, 0.265, 0.337), steel)

    # Shallow pitched roof, modeled as a closed six-vertex prism.
    mesh_part(
        "Pitched shed roof",
        [
            (0.195, -0.225, 0.332), (0.620, -0.225, 0.332),
            (0.4075, -0.225, 0.405),
            (0.195, 0.285, 0.332), (0.620, 0.285, 0.332),
            (0.4075, 0.285, 0.405),
        ],
        [
            (0, 1, 2), (5, 4, 3), (0, 3, 4, 1),
            (1, 4, 5, 2), (2, 5, 3, 0),
        ],
        steel,
    )
    box("Shed door", (0.320, -0.214, 0.098), (0.493, -0.206, 0.285), dark)
    box("Door handle", (0.467, -0.224, 0.180), (0.479, -0.214, 0.214), steel)

    # Exactly one warm accent component.
    accent = box(
        "Amber identification plate",
        (0.355, -0.219, 0.246), (0.458, -0.214, 0.269),
        amber,
    )
    accent_face_count = len(accent.data.polygons)

    # Consolidate all modeled components into exactly one mesh object.
    bpy.ops.object.select_all(action="DESELECT")
    for obj in parts:
        obj.select_set(True)
    bpy.context.view_layer.objects.active = parts[0]
    bpy.ops.object.join()
    model = bpy.context.object
    model.name = "Ore_Mining_Derrick"
    model.data.name = "Ore_Mining_Derrick_Mesh"
    bpy.ops.object.transform_apply(location=True, rotation=True, scale=True)

    for polygon in model.data.polygons:
        polygon.use_smooth = False
    for layer in list(model.data.uv_layers):
        model.data.uv_layers.remove(layer)

    bpy.context.view_layer.update()
    coordinates = [model.matrix_world @ vertex.co for vertex in model.data.vertices]
    if not coordinates or any(
        not math.isfinite(component)
        for coordinate in coordinates for component in coordinate
    ):
        raise RuntimeError("Invalid mesh coordinates")

    minimum_z = min(p.z for p in coordinates)
    maximum_z = max(p.z for p in coordinates)
    height = maximum_z - minimum_z
    radius = max(math.hypot(p.x, p.y) for p in coordinates)
    model.data.calc_loop_triangles()
    triangles = len(model.data.loop_triangles)

    if radius > MAX_RADIUS:
        raise RuntimeError(f"Footprint radius {radius:.9f} exceeds {MAX_RADIUS} m")
    if minimum_z != 0.0:
        raise RuntimeError(f"Minimum Z must be exactly 0.0; got {minimum_z!r}")
    if abs(height - TARGET_HEIGHT) > 1e-6:
        raise RuntimeError(f"Height {height:.9f} differs from {TARGET_HEIGHT} m")
    if triangles >= TRIANGLE_LIMIT:
        raise RuntimeError(f"Triangle count {triangles} must be under {TRIANGLE_LIMIT}")
    if len(scene.objects) != 1 or model.type != "MESH":
        raise RuntimeError("Scene must contain exactly one mesh object")
    if model.modifiers or model.data.uv_layers:
        raise RuntimeError("Unexpected modifiers or UV layers")
    if scene.world is not None:
        raise RuntimeError("Scene must have no world")
    amber_faces = sum(
        1 for p in model.data.polygons
        if model.data.materials[p.material_index] == amber
    )
    if amber_faces != accent_face_count:
        raise RuntimeError("Amber must appear only on the single accent plate")

    os.makedirs(os.path.dirname(path), exist_ok=True)
    print(f"Ore mining derrick: triangles={triangles}, height={height:.6f} m", flush=True)
    result = bpy.ops.export_scene.gltf(
        filepath=path,
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


if __name__ == "__main__":
    main()
