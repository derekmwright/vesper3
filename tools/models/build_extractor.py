import math
import sys
from pathlib import Path

import bpy
from mathutils import Vector


TARGET_HEIGHT = 1.10
MAX_RADIUS = 0.80
TRIANGLE_LIMIT = 1500
TOLERANCE = 1e-6


def output_path():
    if "--" not in sys.argv:
        raise RuntimeError("Usage: blender --background --python script.py -- out.glb")
    arguments = sys.argv[sys.argv.index("--") + 1:]
    if not arguments or not arguments[-1].strip():
        raise RuntimeError("Missing output path after --")
    path = Path(arguments[-1]).expanduser().resolve()
    if path.suffix.lower() != ".glb":
        raise RuntimeError("Output path must end in .glb")
    path.parent.mkdir(parents=True, exist_ok=True)
    return str(path)


def material(name, color, metallic, roughness):
    result = bpy.data.materials.new(name)
    result.use_nodes = True
    result.diffuse_color = (*color, 1.0)
    shader = result.node_tree.nodes.get("Principled BSDF")
    shader.inputs["Base Color"].default_value = (*color, 1.0)
    shader.inputs["Metallic"].default_value = metallic
    shader.inputs["Roughness"].default_value = roughness
    return result


def lathe(name, profile, segments, surface, axis="Z", center_z=0.0):
    """Create a capped solid from ascending (axial position, radius) pairs."""
    vertices = []
    faces = []
    rings = []

    for position, radius in profile:
        ring = []
        count = 1 if radius == 0.0 else segments
        for index in range(count):
            angle = 2.0 * math.pi * index / segments
            a = radius * math.cos(angle)
            b = radius * math.sin(angle)
            if axis == "Z":
                coordinate = (a, b, position)
            elif axis == "X":
                coordinate = (position, a, center_z + b)
            else:
                raise RuntimeError("Unsupported lathe axis")
            ring.append(len(vertices))
            vertices.append(coordinate)
        rings.append(ring)

    if len(rings[0]) > 1:
        faces.append(tuple(reversed(rings[0])))

    for lower, upper in zip(rings, rings[1:]):
        if len(lower) == 1 and len(upper) == 1:
            raise RuntimeError("Adjacent profile points cannot both be poles")
        for index in range(segments):
            following = (index + 1) % segments
            if len(upper) == 1:
                faces.append((lower[index], lower[following], upper[0]))
            elif len(lower) == 1:
                faces.append((lower[0], upper[following], upper[index]))
            else:
                faces.append((
                    lower[index], lower[following],
                    upper[following], upper[index],
                ))

    if len(rings[-1]) > 1:
        faces.append(tuple(rings[-1]))

    mesh = bpy.data.meshes.new(name + "_Mesh")
    mesh.from_pydata(vertices, [], faces)
    mesh.update()
    obj = bpy.data.objects.new(name, mesh)
    bpy.context.scene.collection.objects.link(obj)
    mesh.materials.append(surface)
    for polygon in mesh.polygons:
        polygon.use_smooth = False
    return obj


def main():
    destination = output_path()

    # Remove all existing objects, including hidden/default scene objects.
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

    metal = material("Cool_Blue_Grey_Metal", (0.27, 0.39, 0.46), 0.65, 0.48)
    amber = material("Warm_Amber_Collar", (0.95, 0.34, 0.045), 0.25, 0.40)

    parts = []

    # Low octagonal pad; its underside establishes the exact ground plane.
    parts.append(lathe(
        "Octagonal_Base",
        [(0.0, 0.56), (0.025, 0.60), (0.085, 0.60), (0.11, 0.56)],
        8, metal,
    ))

    # Broad insulated cylinder with a faceted hemispherical dome.
    dome_start = 0.67
    tank_radius = 0.43
    tank_profile = [
        (0.095, 0.39),
        (0.14, tank_radius),
        (dome_start, tank_radius),
    ]
    for degrees in (22.5, 45.0, 67.5):
        angle = math.radians(degrees)
        tank_profile.append((
            dome_start + tank_radius * math.sin(angle),
            tank_radius * math.cos(angle),
        ))
    tank_profile.append((TARGET_HEIGHT, 0.0))
    parts.append(lathe("Insulated_Tank", tank_profile, 16, metal))

    # A shallow metal footing reinforces the tank's lower edge.
    parts.append(lathe(
        "Tank_Footing",
        [(0.105, 0.435), (0.13, 0.45), (0.19, 0.45), (0.21, 0.435)],
        16, metal,
    ))

    # Short horizontal intake. The return along the inner wall forms a
    # recessed, closed-bottom opening without textures or extra materials.
    intake_profile = [
        (0.35, 0.088),
        (0.65, 0.088),
        (0.69, 0.103),
        (0.72, 0.103),
        (0.72, 0.073),
        (0.60, 0.073),
    ]
    parts.append(lathe(
        "Intake_Pipe", intake_profile, 12, metal, axis="X", center_z=0.48,
    ))

    # Exactly one warm accent detail: a single collar around the intake.
    parts.append(lathe(
        "Amber_Intake_Collar",
        [(0.555, 0.092), (0.565, 0.107), (0.600, 0.107), (0.610, 0.092)],
        12, amber, axis="X", center_z=0.48,
    ))

    bpy.ops.object.select_all(action="DESELECT")
    for obj in parts:
        obj.select_set(True)
    bpy.context.view_layer.objects.active = parts[0]
    bpy.ops.object.join()

    model = bpy.context.object
    model.name = "Ice_Extractor"
    model.data.name = "Ice_Extractor_Mesh"
    scene.cursor.location = (0.0, 0.0, 0.0)
    bpy.ops.object.origin_set(type="ORIGIN_CURSOR")
    bpy.ops.object.transform_apply(location=False, rotation=True, scale=True)

    mesh = model.data
    for polygon in mesh.polygons:
        polygon.use_smooth = False
    for layer in list(mesh.uv_layers):
        mesh.uv_layers.remove(layer)
    mesh.update()
    mesh.calc_loop_triangles()

    coordinates = [model.matrix_world @ vertex.co for vertex in mesh.vertices]
    if not coordinates or any(
        not math.isfinite(value) for coordinate in coordinates for value in coordinate
    ):
        raise RuntimeError("Mesh contains missing or invalid coordinates")

    radius = max(math.hypot(point.x, point.y) for point in coordinates)
    minimum_z = min(point.z for point in coordinates)
    maximum_z = max(point.z for point in coordinates)
    height = maximum_z - minimum_z
    triangles = len(mesh.loop_triangles)

    if radius > MAX_RADIUS:
        raise RuntimeError(f"Footprint radius {radius:.9f} exceeds {MAX_RADIUS} m")
    if minimum_z != 0.0:
        raise RuntimeError(f"Minimum Z must be exactly 0.0; got {minimum_z!r}")
    if abs(height - TARGET_HEIGHT) > TOLERANCE:
        raise RuntimeError(f"Height {height:.9f} differs from {TARGET_HEIGHT} m")
    if triangles >= TRIANGLE_LIMIT:
        raise RuntimeError(f"Triangle count {triangles} must be below {TRIANGLE_LIMIT}")
    if len(scene.objects) != 1 or model.type != "MESH":
        raise RuntimeError("Scene must contain exactly one mesh object")
    if mesh.uv_layers or model.modifiers:
        raise RuntimeError("Model must have no UVs or modifiers")
    if scene.world is not None:
        raise RuntimeError("Scene must have no world")

    print(f"Ice_Extractor: triangles={triangles}, height={height:.6f} m", flush=True)

    bpy.ops.export_scene.gltf(
        filepath=destination,
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


if __name__ == "__main__":
    main()
