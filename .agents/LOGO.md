# Memex — Elephant Logo: Artistic Decisions

This document records every design decision behind the cubist elephant logo.
It is the single source of truth for any agent or human continuing work on it.
Read it entirely before touching any logo file.

The cubist elephant is the visual identity of this repo. It is not a placeholder.


## Origin and intent

The original logo was a Lucide brain icon — generic, replaceable, meaningless.
The owner decided the project needed its own identity. After discarding
hats, bowties, and the brain-only path, the direction became clear:
a cubist elephant with a visible brain inside. An animal known for memory,
rendered through the visual language of Picasso and Miró.

The elephant is not a mascot. It is the logo. It will appear in the README,
the docs, and anywhere the project is represented.


## Visual philosophy

Two principles govern every decision:

**Geometries define planes. Curves define what is alive.**

Structural elements — skull, forehead, jaw, ear — are polygons. Hard edges,
flat fills, deliberate faceting. This is the cubist layer.

Organic elements — trunk, ear inner plane, brain scribble — use curves.
They have life. They are not perfect. They should feel drawn, not computed.

**The elephant must be immediately recognizable from the geometry alone.**

If you removed all color and left only the outlines, you should still see
an elephant. The composition of planes does the work, not the detail.


## Canvas and technical setup

- viewBox: `0 0 120 120`
- Export size: `400 × 400` (width/height attributes)
- Global stroke: `#2d1f2a` (near-black with warm magenta undertone)
- stroke-linecap: `round`
- stroke-linejoin: `round`
- fill: `none` at SVG root level (each element defines its own fill)


## Color palette

### Elephant

| Role                  | Hex       | Notes                                  |
|-----------------------|-----------|----------------------------------------|
| Skull (bright plane)  | `#fce8ef` | Palest pink, the lightest face plane   |
| Skull (mid plane)     | `#fff0f3` | Forehead/sien                          |
| Face fill (central)   | `#fce8ef` | Gap-filling polygon between head planes|
| Nuca / side           | `#f4a0b8` | Mid pink, posterior head plane         |
| Ear outer             | `#e8829e` | Darker pink, outer ear polygon         |
| Ear inner             | `#fbb6ce` | Medium pastel pink, inner ear plane    |
| Trunk/snout           | `#fbb6ce` | Same as ear inner — organic continuity |
| Jaw                   | `#e8829e` | Same as ear outer — grounds the face   |
| Brain ellipse bg      | `#fbb6ce` | Soft backdrop for the scribble         |
| Brain scribble        | stroke only, `#2d1f2a` |                             |
| Eye socket            | `#2d1f2a` | Diamond polygon, filled solid          |
| Eye highlight         | `#fff0f3` | Small circle                           |
| Eye pupil             | `#2d1f2a` | Smaller circle                         |

### Background

| Color      | Hex       |
|------------|-----------|
| Yellow     | `#f9e04b` |
| Gold       | `#f4c430` |
| Green      | `#7bc67e` |
| Dark green | `#4caf50` |
| Sky blue   | `#a8d8ea` |
| Blue       | `#4a90d9` |
| Coral red  | `#e8534a` |

The background rect is `#4caf50` (dark green). The 8 large polygons sit on top
and tile the canvas completely — no transparent gaps.

**Color placement rule:** treat background polygons as a graph coloring problem.
Map which polygons share a border, then assign colors so no two adjacent polygons
share the same hue family. Never assign by geographic zone (e.g. "blues on the
right") — distribute without spatial logic so the result feels cubist, not mapped.


## Structure: layers in render order

### 1. Background
A full-canvas rect plus 8 large polygons that cover every pixel.
Polygon count is deliberately low — large shapes, not many small ones.
The background should feel like a stained-glass window, not confetti.

### 2. Ear
Two polygons. The outer one is larger and darker (`#e8829e`).
The inner one is smaller and lighter (`#fbb6ce`), slightly inset.
The ear extends to the left, behind the head planes.

### 3. Head
Three polygons forming the skull from back to front:
- **Cráneo** (top of skull): `#fce8ef`, lightest plane
- **Nuca** (posterior/side): `#f4a0b8`, mid-tone, behind the forehead
- **Frente/sien** (forehead): `#fff0f3`, rightmost head plane

Stroke widths on head planes: 2.2–2.4. Thicker than the background (1.0)
to assert the elephant over the geometry behind it.

The three head polygons do not perfectly tile — they leave small gaps where
the background shows through. These gaps are covered by additional small
fill polygons inserted **before** the trunk in render order:
- Central face gap: `56,36  76,32  82,46  80,54  58,58` — `#fce8ef`
- Gap above trunk: `82,46  94,50  80,54` — `#fce8ef`

**Do not modify existing head polygon vertices to close gaps.** Add fill
polygons instead. The silhouette must not change.

### 4. Gap fill polygons
Small polygons that close the holes between head planes. Same fill as the
lightest skull plane (`#fce8ef`). Stroke width 1.8. Added between the head
section and the trunk section in the SVG, so they render above the head
but below the trunk.

### 5. Trunk + snout (morro + trompa)
A single `<path>` combining both. The snout transitions into the trunk
without interruption — same fill, same shape, one continuous element.
The trunk uses cubic bezier curves. It curls downward and ends with a
small curl at the tip. It does not look like a separate attachment.

Stroke width: 2.2.

Below the trunk, the jaw polygon (`#e8829e`) closes the base of the face.

### 6. Brain
See dedicated section below.

### 7. Eye
A diamond `<polygon>` filled solid (`#2d1f2a`), with two concentric circles
on top: a light highlight and a dark pupil. Simple, graphic, effective.
Position: approximately (90, 22) — in the forehead/sien plane.


## Brain: the scribble

The brain is not a polygon. It is not a set of geometric shapes.
It is a single continuous cubic bezier path that never lifts from the paper.

### Concept
Imagine someone signing their name with full abandon — the pen moves in
rough loops that drift across the page, each loop landing somewhere different
from the last, crossing back over previous strokes, never concentric,
never rhythmic. That is the brain.

### Rules
- One `<path>`, no `fill`, only `stroke`
- `stroke-width`: 1.4
- `stroke-linecap`: round
- `stroke-linejoin`: round
- All curves are cubic beziers (`C`). No straight lines (`L`), no arcs.
- No two loops share the same center. Each one drifts in a different direction.
- The loops are not circular — they are deformed ovals, irregular, alive.
- The path crosses itself multiple times. That is intentional.
- The scribble must stay within the skull plane, not touching the eye,
  the ear lines, or the forehead polygon edges.

### Placement (canonical)
- Backdrop ellipse: `cx="64" cy="18" rx="9" ry="7"`, fill `#fbb6ce`
- The path occupies roughly the zone `x: 54–76, y: 8–27`
- Centered in the upper-left portion of the skull, away from the eye

### What to avoid
- Concentric loops (looks like a spiral, not a scribble)
- Loops that repeat the same radius
- Any straight segment
- The scribble touching or overlapping the eye diamond or the ear polygon edges
- Making it too large — it should read as "brain inside skull", not "chaos filling the head"


## Version history (relevant milestones)

| Version    | Status       | Notes                                                          |
|------------|--------------|----------------------------------------------------------------|
| v1–v6      | Discarded    | Early experiments, Miró phase, hat+bowtie attempts             |
| v7–v10     | Discarded    | Cubist direction established but not balanced                  |
| v11        | Reference    | First version that worked. Established all core decisions.     |
| v12–v14    | Discarded    | Various failed experiments                                     |
| v15–v19    | Discarded    | Brain scribble experiments — too geometric, too concentric     |
| v20        | Breakthrough | First scribble with non-concentric drifting loops              |
| v21        | Very good    | More dispersed loops, wider drift                              |
| v22        | Base         | v21 shifted -3x, -3y to clear surrounding shapes              |
| logo.svg   | **CANONICAL**| v22 + coral red background + graph-colored bg + gap fills      |


## What to preserve in future iterations

These decisions are settled. Do not revisit them without explicit owner instruction:

1. The elephant is cubist. Structural planes are polygons. No smooth outlines for the head.
2. The trunk is a single organic path continuous with the snout. Never two separate elements.
3. The background covers the full canvas with large colored polygons. No transparent gaps.
4. The brain is a free-form scribble, single path, no fill. Never geometric shapes.
5. Background colors must be graph-colored — no two adjacent polygons share a hue family.
6. The color palette stays in the pink/warm range for the elephant body.
7. The eye is a diamond polygon with two concentric circles. Simple and graphic.
8. Stroke color is always `#2d1f2a`. Never pure black, never grey.
9. The elephant must be recognizable as an elephant at a glance. If it isn't, it's wrong.
10. Never modify existing polygon vertices to fix gaps — add small fill polygons instead.


## Files

All logo files live in `docs/images/`.

| File         | Status                                      |
|--------------|---------------------------------------------|
| `logo.svg`   | **Canonical. The official logo. Do not overwrite without owner approval.** |

When iterating: work on a separate `logo-wip.svg`. Once approved, overwrite
`logo.svg` and delete the wip file. Never keep iteration files in the repo.
