import bpy
import math
import sys
from pathlib import Path


def output_path():
    if "--" not in sys.argv:
        raise RuntimeError("Usage: blender --background --python script.py -- out.glb")
    arguments = sys.argv[sys.argv.index("--") + 1:]
    if not arguments:
        raise RuntimeError("Missing output path after --.")
    path = Path(arguments[-1]).expanduser().resolve()
    if path.suffix.lower() != ".glb":
        raise RuntimeError("Output path must end in .glb.")
    return path


def material(name, color, metallic, roughness):
    mat = bpy.data.materials.new(name)
    mat.diffuse_color = (*color, 1.0)
    mat.use_nodes = True
    bsdf = mat.node_tree.nodes.get("Principled BSDF")
    bsdf.inputs["Base Color"].default_value = (*color, 1.0)
    bsdf.inputs["Metallic"].default_value = metallic
    bsdf.inputs["Roughness"].default_value = roughness
    return mat


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
    scene.unit_settings.scale_length = 1.0
    scene.unit_settings.length_unit = "METERS"

    materials = [
        material("Cool blue-grey metal", (0.24, 0.34, 0.42), 0.65, 0.48),
        material("Dark blue-grey metal", (0.075, 0.12, 0.16), 0.55, 0.62),
        material("Warm amber collar", (0.95, 0.38, 0.045), 0.35, 0.40),
    ]
    parts = []

    def mesh_part(name, vertices, faces, material_index=0):
        mesh = bpy.data.meshes.new(name)
        mesh.from_pydata(vertices, [], faces)
        mesh.update()
        mesh.materials.append(materials[material_index])
        for polygon in mesh.polygons:
            polygon.use_smooth = False
        obj = bpy.data.objects.new(name, mesh)
        scene.collection.objects.link(obj)
        parts.append(obj)
        return obj

    def lathe(name, center, profile, sides, material_index=0, caps=True):
        # Profile entries are (radius, absolute Z), ordered along the surface.
        vertices = []
        for radius, z in profile:
            for i in range(sides):
                angle = 2.0 * math.pi * i / sides
                vertices.append((
                    center[0] + radius * math.cos(angle),
                    center[1] + radius * math.sin(angle),
                    z,
                ))
        faces = []
        for level in range(len(profile) - 1):
            for i in range(sides):
                j = (i + 1) % sides
                a = level * sides
                b = (level + 1) * sides
                faces.append((a + i, a + j, b + j, b + i))
        if caps:
            faces.append(tuple(reversed(range(sides))))
            offset = (len(profile) - 1) * sides
            faces.append(tuple(offset + i for i in range(sides)))
        return mesh_part(name, vertices, faces, material_index)

    def box(name, center, size, material_index=0):
        x, y, z = center
        a, b, c = (value / 2.0 for value in size)
        vertices = [
            (x-a, y-b, z-c), (x+a, y-b, z-c),
            (x+a, y+b, z-c), (x-a, y+b, z-c),
            (x-a, y-b, z+c), (x+a, y-b, z+c),
            (x+a, y+b, z+c), (x-a, y+b, z+c),
        ]
        faces = [
            (3, 2, 1, 0), (4, 5, 6, 7),
            (0, 1, 5, 4), (1, 2, 6, 5),
            (2, 3, 7, 6), (3, 0, 4, 7),
        ]
        return mesh_part(name, vertices, faces, material_index)

    # Octagonal foundation with a small bevel; bottom vertices are exactly Z=0.
    lathe("Octagonal foundation", (0.0, 0.0),
          [(0.76, 0.0), (0.76, 0.065), (0.73, 0.09)], 8, 1)

    hall_center = (-0.12, 0.0)
    lathe("Turbine hall footing", hall_center,
          [(0.50, 0.075), (0.50, 0.15)], 16, 1)
    lathe("Squat cylindrical turbine hall", hall_center,
          [(0.475, 0.12), (0.475, 0.61)], 16)
    lathe("Faceted turbine roof", hall_center,
          [(0.495, 0.60), (0.495, 0.65), (0.41, 0.71)], 16)
    lathe("Roof service hatch", hall_center,
          [(0.17, 0.705), (0.17, 0.745)], 8, 1)

    # A single narrow stack rises alongside and intersects the turbine hall.
    stack_center = (0.43, 0.13)
    lathe("Stack plinth", stack_center,
          [(0.18, 0.075), (0.18, 0.20), (0.145, 0.25)], 8, 1)
    lathe("Exhaust stack", stack_center,
          [(0.125, 0.17), (0.125, 0.78),
           (0.102, 1.32), (0.102, 1.40),
           (0.073, 1.40), (0.073, 1.24)], 12, caps=False)
    lathe("Recessed stack interior", stack_center,
          [(0.073, 1.235), (0.073, 1.24)], 12, 1)

    # Exactly one amber detail: a continuous collar around the stack.
    lathe("Single amber identification collar", stack_center,
          [(0.112, 1.13), (0.109, 1.21)], 12, 2)

    # Front maintenance entrance and restrained industrial ventilation.
    box("Maintenance door frame", (-0.12, -0.469, 0.285),
        (0.235, 0.055, 0.33), 1)
    box("Maintenance door", (-0.12, -0.502, 0.285),
        (0.19, 0.02, 0.285))
    box("Door handle", (-0.052, -0.519, 0.285),
        (0.014, 0.016, 0.065), 1)
    for i in range(3):
        box("Front ventilation slat %d" % i,
            (-0.12, -0.478, 0.495 + i * 0.035),
            (0.25, 0.035, 0.014), 1)

    # Join every component into one mesh, with identity object transforms.
    bpy.ops.object.select_all(action="DESELECT")
    for obj in parts:
        obj.select_set(True)
    bpy.context.view_layer.objects.active = parts[0]
    bpy.ops.object.join()
    obj = bpy.context.object
    obj.name = "Geothermal_Power_Plant"
    scene.cursor.location = (0.0, 0.0, 0.0)
    bpy.ops.object.origin_set(type="ORIGIN_CURSOR")
    bpy.ops.object.transform_apply(location=True, rotation=True, scale=True)

    mesh = obj.data
    for layer in list(mesh.uv_layers):
        mesh.uv_layers.remove(layer)
    for polygon in mesh.polygons:
        polygon.use_smooth = False

    # Validate actual joined geometry in world-space metres.
    bpy.context.view_layer.update()
    points = [obj.matrix_world @ vertex.co for vertex in mesh.vertices]
    if not points:
        raise RuntimeError("The model contains no vertices.")
    if any(not math.isfinite(value) for point in points for value in point):
        raise RuntimeError("The model contains non-finite coordinates.")

    radius = max(math.hypot(point.x, point.y) for point in points)
    minimum_z = min(point.z for point in points)
    maximum_z = max(point.z for point in points)
    height = maximum_z - minimum_z
    mesh.calc_loop_triangles()
    triangles = len(mesh.loop_triangles)

    if radius > 0.80:
        raise RuntimeError("Footprint radius exceeds 0.80 m: %.9f" % radius)
    if minimum_z != 0.0:
        raise RuntimeError("Minimum Z must be exactly 0.0: %.9f" % minimum_z)
    if abs(height - 1.40) > 0.00001:
        raise RuntimeError("Height must be 1.40 m: %.9f" % height)
    if not 0 < triangles < 1500:
        raise RuntimeError("Triangle count must be under 1500: %d" % triangles)
    if len(scene.objects) != 1 or obj.type != "MESH":
        raise RuntimeError("The scene must contain exactly one mesh object.")
    if mesh.uv_layers or obj.modifiers:
        raise RuntimeError("Unexpected UV layers or modifiers.")
    if scene.world is not None:
        raise RuntimeError("The scene must have no world.")

    destination.parent.mkdir(parents=True, exist_ok=True)
    print("Geothermal power plant: %d triangles, height %.6f m"
          % (triangles, height), flush=True)

    # Export operator reference:
    # https://docs.blender.org/api/main/bpy.ops.export_scene.html
    result = bpy.ops.export_scene.gltf(
        filepath=str(destination),
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
        check_existing=False,
    )
    if "FINISHED" not in result or not destination.is_file():
        raise RuntimeError("GLB export failed.")


if __name__ == "__main__":
    main()
