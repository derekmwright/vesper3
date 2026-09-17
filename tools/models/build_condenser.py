import bpy
import bmesh
import math
import sys
from pathlib import Path

# Export API reference:
# https://docs.blender.org/api/main/bpy.ops.export_scene.html

TARGET_HEIGHT = 1.05
MAX_RADIUS = 0.80
MAX_TRIANGLES = 1500
HEIGHT_TOLERANCE = 1e-6


def output_path():
    if "--" not in sys.argv:
        raise RuntimeError("Usage: blender --background --python script.py -- out.glb")
    arguments = sys.argv[sys.argv.index("--") + 1:]
    if not arguments:
        raise RuntimeError("Missing output path after --.")
    path = Path(arguments[-1]).expanduser().resolve()
    if path.suffix.lower() != ".glb":
        raise RuntimeError("Output path must end in .glb.")
    path.parent.mkdir(parents=True, exist_ok=True)
    return str(path)


def material(name, color, metallic, roughness):
    mat = bpy.data.materials.new(name)
    mat.diffuse_color = (*color, 1.0)
    mat.use_nodes = True
    shader = mat.node_tree.nodes.get("Principled BSDF")
    shader.inputs["Base Color"].default_value = (*color, 1.0)
    shader.inputs["Metallic"].default_value = metallic
    shader.inputs["Roughness"].default_value = roughness
    return mat


parts = []


def mesh_part(name, vertices, faces, mat):
    mesh = bpy.data.meshes.new(name)
    mesh.from_pydata(vertices, [], faces)
    mesh.update()
    obj = bpy.data.objects.new(name, mesh)
    bpy.context.scene.collection.objects.link(obj)
    mesh.materials.append(mat)
    parts.append(obj)
    return obj


def lathe(name, rings, sides, mat, apex=None):
    """Closed polygonal solid; rings contain (radius, z) pairs."""
    vertices = []
    for radius, z in rings:
        for i in range(sides):
            angle = 2.0 * math.pi * i / sides
            vertices.append((radius * math.cos(angle),
                             radius * math.sin(angle), z))

    faces = [tuple(reversed(range(sides)))]
    for row in range(len(rings) - 1):
        lower = row * sides
        upper = lower + sides
        for i in range(sides):
            j = (i + 1) % sides
            faces.append((lower + i, lower + j, upper + j, upper + i))

    top = (len(rings) - 1) * sides
    if apex is None:
        faces.append(tuple(top + i for i in range(sides)))
    else:
        tip = len(vertices)
        vertices.append((0.0, 0.0, apex))
        for i in range(sides):
            faces.append((top + i, top + (i + 1) % sides, tip))

    return mesh_part(name, vertices, faces, mat)


def radiator_fin(index, mat):
    # Broad, flat radial blade with clipped outer corners.
    profile = [
        (0.105, 0.17),
        (0.295, 0.17),
        (0.355, 0.23),
        (0.355, 0.80),
        (0.300, 0.86),
        (0.105, 0.86),
    ]
    angle = index * math.tau / 6.0
    ca, sa = math.cos(angle), math.sin(angle)
    vertices = []
    for tangent in (-0.012, 0.012):
        for radius, z in profile:
            vertices.append((radius * ca - tangent * sa,
                             radius * sa + tangent * ca, z))

    n = len(profile)
    faces = [tuple(reversed(range(n))), tuple(range(n, 2 * n))]
    for i in range(n):
        j = (i + 1) % n
        faces.append((i, j, n + j, n + i))
    return mesh_part("Radiator_Fin_%02d" % (index + 1), vertices, faces, mat)


def main():
    destination = output_path()

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
    scene.unit_settings.length_unit = "METERS"
    scene.unit_settings.scale_length = 1.0

    metal = material("Cool_Blue_Grey_Metal", (0.23, 0.34, 0.41), 0.75, 0.48)
    amber = material("Warm_Amber_Accent", (0.95, 0.36, 0.055), 0.25, 0.38)

    lathe("Octagonal_Base_Pad",
          [(0.39, 0.0), (0.39, 0.045), (0.35, 0.075)], 8, metal)
    lathe("Column_Foot",
          [(0.17, 0.065), (0.17, 0.105), (0.12, 0.145)], 12, metal)
    lathe("Slim_Column", [(0.115, 0.10), (0.115, 0.955)], 12, metal)

    for index in range(6):
        radiator_fin(index, metal)

    # Exactly one amber detail: a narrow continuous collar above the fins.
    lathe("Amber_Status_Collar",
          [(0.123, 0.882), (0.123, 0.904)], 12, amber)
    lathe("Cap_Rim", [(0.143, 0.935), (0.143, 0.955)], 12, metal)

    # Three faceted latitude rings and one pole form a small shallow dome.
    dome_rings = []
    for degrees in (0.0, 30.0, 60.0):
        angle = math.radians(degrees)
        dome_rings.append((0.14 * math.cos(angle),
                           0.955 + 0.095 * math.sin(angle)))
    lathe("Dome_Cap", dome_rings, 12, metal, apex=TARGET_HEIGHT)

    bpy.ops.object.select_all(action="DESELECT")
    for obj in parts:
        obj.select_set(True)
    bpy.context.view_layer.objects.active = parts[0]
    bpy.ops.object.join()
    model = bpy.context.object
    model.name = "Atmospheric_Water_Condenser"

    # Explicit triangles give an unambiguous exported triangle budget.
    mesh = model.data
    bm = bmesh.new()
    bm.from_mesh(mesh)
    bmesh.ops.recalc_face_normals(bm, faces=list(bm.faces))
    bmesh.ops.triangulate(bm, faces=list(bm.faces))
    bm.to_mesh(mesh)
    bm.free()

    for polygon in mesh.polygons:
        polygon.use_smooth = False
    for layer in list(mesh.uv_layers):
        mesh.uv_layers.remove(layer)
    mesh.update()
    bpy.context.view_layer.update()

    vertices = [model.matrix_world @ vertex.co for vertex in mesh.vertices]
    if not vertices:
        raise RuntimeError("Generated mesh is empty.")
    if not all(math.isfinite(component) for v in vertices for component in v):
        raise RuntimeError("Generated mesh contains non-finite coordinates.")

    minimum_z = min(v.z for v in vertices)
    maximum_z = max(v.z for v in vertices)
    height = maximum_z - minimum_z
    radius = max(math.hypot(v.x, v.y) for v in vertices)
    mesh.calc_loop_triangles()
    triangle_count = len(mesh.loop_triangles)

    if radius > MAX_RADIUS:
        raise RuntimeError(f"Footprint radius exceeds limit: {radius:.9f} m")
    if minimum_z != 0.0:
        raise RuntimeError(f"Minimum Z must be exactly zero: {minimum_z!r}")
    if abs(height - TARGET_HEIGHT) > HEIGHT_TOLERANCE:
        raise RuntimeError(f"Height must be approximately 1.05 m: {height:.9f}")
    if not 0 < triangle_count < MAX_TRIANGLES:
        raise RuntimeError(f"Triangle count must be under 1500: {triangle_count}")
    if len(scene.objects) != 1 or model.type != "MESH":
        raise RuntimeError("Scene must contain exactly one mesh object.")
    if model.modifiers or mesh.uv_layers:
        raise RuntimeError("Unexpected modifiers or UV layers.")

    print(f"Triangles: {triangle_count} | Height: {height:.6f} m", flush=True)

    result = bpy.ops.export_scene.gltf(
        filepath=destination,
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
    if "FINISHED" not in result:
        raise RuntimeError(f"glTF export failed: {result}")


if __name__ == "__main__":
    main()
