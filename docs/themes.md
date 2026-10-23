# Themes

A theme changes the office's colors, how its people, its furniture and its
pet look, and the colors of the windows and terminals. It is a folder
of plain text files on your machine: nothing is downloaded, and you can share
a theme by copying its folder.

It covers the palettes, the colors of the room, the furniture and the pet,
the characters' poses, the shapes of the furniture and of the pet's corner,
the windows (terminals, board, settings) and the terminals' 16 ANSI colors.
What tells you something rather than decorates the office stays the same in
every theme: the status colors and the speech bubbles over agents and the
pet.

## Themes that come with the office

Besides `default`, the office brings `night` (dark room, bright agents),
`retro` (warm walls, teal floor, 80s colors) and `gentle-ai` (the Gentleman
theme of [Gentleman.Dots](https://github.com/Gentleman-Programming/Gentleman.Dots),
the one gentle-ai is used with: its terminal palette, its background and text
in the windows, and its colors on the agents). Pick one in the hub's settings tab
(`s`) or in `config.toml`:

```toml
[theme]
name = "night"
```

These names are taken: a theme folder cannot use them.

## Wear another theme's colors

A theme is shapes and colors. `palette` keeps the shapes of the theme in
`name` and dresses them in the colors of another theme:

```toml
[theme]
name    = "my-aliens"   # its people, poses and room
palette = "gentle-ai"   # its colors on top
```

Every color the palette's `theme.toml` lists replaces the theme's; colors it
leaves out stay the theme's own. `gentle-ai`, for instance, lists the room,
the agents, the windows and the terminal, so green alien skin stays green.
A palette can be any theme that comes with the office or one in the themes
folder, except `default`: the built-in theme is the base under every theme
and lists no colors of its own. Without `palette`, a theme wears its own
colors.

In the hub's settings tab, `c` on a theme makes it the palette and saves
it; `c` on `default (built-in)` goes back to the theme's own colors.

## Start from the built-in theme

In the office, `s` opens the hub's settings tab: pick a theme and press `enter`
to apply and save it, or press `n` to copy the active theme under a new name.
From a terminal:

```sh
bah theme init my-theme
```

This writes the built-in theme, every color, every pose, every piece of
furniture and every pet file, to the themes
folder next to your `config.toml`:

| OS | Folder |
|---|---|
| Windows | `%APPDATA%\bitagentshub\themes\my-theme\` |
| macOS | `~/Library/Application Support/bitagentshub/themes/my-theme/` |
| Linux | `~/.config/bitagentshub/themes/my-theme/` (or under `$XDG_CONFIG_HOME`) |

To start from another theme, name it with `--from`: one that comes with the
office or one already in the themes folder. `gentle-ai` is the most complete
starting point, since it sets the window colors and the 16 terminal colors
too:

```sh
bah theme init my-theme --from gentle-ai
```

It refuses to write into a folder that already has files. Then select the theme in
`config.toml` (same folder as `themes`) and restart the office:

```toml
[theme]
name = "my-theme"
```

Without a `[theme]` section the built-in theme is used.

## An example: a spaceship

[`examples/themes/space/`](../examples/themes/space/) turns the office into
a spaceship: aliens with glowing antennae at the consoles, hull plates with
a light strip for walls and floor, portholes to the stars, a violet plant
and a three-eyed cat. It is only files, no code:

- `theme.toml` gives the colors (green, violet and cyan skin, metal, space)
  and two colors of its own, `antenna` and `hull_light`, with the letters
  `Z` and `Y` to draw them.
- `characters/` redraws every pose: the hair rows become two antennae with
  a `Z` on top.
- `pet/` adds a third `E` to every frame of the cat.
- `room/` and `furniture/window.txt` and `furniture/plant.txt` redraw the
  tiles, the windows and the plant; every other piece comes from the
  built-in theme.

Copy the folder into your themes folder and select it:

```toml
[theme]
name = "space"
```

`palette` works on it like on any theme: `palette = "gentle-ai"` keeps the
spaceship and the aliens and dresses them in gentle-ai's colors.

## Make a theme with your agent

This page is the whole format, so an agent can write a theme from it. Give
it this file and the folder `bah theme init` wrote, and ask for what you
want, for instance:

> Read docs/themes.md. In my theme folder, turn the office into a pirate
> ship: the people are pirates with bandanas, the floor is deck planks, the
> windows are portholes and the pet is a parrot-colored cat. Keep every
> piece's size.

Then restart the office: a file that breaks a rule stops it with an error
naming the file, the line and what it needs, which you can hand back to the
agent as it is.

## The folder

```
my-theme/
  theme.toml
  characters/
    walk-down.txt  walk-up.txt  walk-side.txt
    stand-down.txt stand-up.txt stand-side.txt
    typing.txt     reading.txt  thinking.txt
    blocked.txt    asleep.txt   lounging.txt
  room/
    wall.txt       floor.txt
  furniture/
    desk.txt       chair.txt    window.txt   painting.txt clock.txt
    bookshelf.txt  coffee.txt   cooler.txt   sofa.txt     plant.txt
  pet/
    sleeping.txt   busy.txt     tired.txt    exhausted.txt
    hungry.txt     alert.txt    eating.txt
    bed.txt        bowl.txt
```

`theme.toml` is required; everything in it and every pose, room, furniture
and pet file is optional. Whatever a theme leaves out, a color, a whole pose,
a room tile, a piece of furniture or a pet file, comes from the built-in
theme, so a theme can be a `theme.toml` with one color. Only the files listed
above may sit in `characters/`, `room/`, `furniture/` and `pet/` (other
`.txt` files are an error;
files with other extensions are ignored).

## theme.toml

The values below are an example; `theme init` writes the built-in ones.

```toml
name = "My theme"
author = "you"

[palette]
hair   = ["#2b1d14", "#d9a066"]   # people get one of these, picked per agent
skin   = ["#f2c9a0", "#8d5524"]
pants  = ["#3b4252", "#5e4530"]
agents = ["#61afef", "#e06c75"]   # shirt and identity color of each agent
eye        = "#22222a"
shoes      = "#24201c"
paper      = "#e5e9f0"            # the page an agent reads
paper_line = "#9aa0aa"

[room]
wall_top   = "#3b4252"
wall_fill  = "#4c566a"
wall_panel = "#465063"
baseboard  = "#2e3440"
floor_a    = "#6b4f3a"   # the two plank colors
floor_b    = "#634935"
floor_seam = "#4e3a2b"
carpets    = ["#3d5a6c", "#5c4a6b"]   # one per area, in turn

[furniture]
desk_top   = "#c8a272"
desk_front = "#8b6a45"
desk_leg   = "#5c4630"
chair_back = "#434c5e"
chair_edge = "#2e3440"
bezel      = "#2b303b"   # the monitor's frame
screen_off = "#15181e"   # the monitor while its agent is idle
mug        = "#e5e9f0"   # the mug on the desk and on the coffee machine
coffee     = "#6b4226"
# What a monitor shows, by what its agent does. Each ink list cycles down
# the lines of text on the screen.
screen_code     = "#1d2b45"   # writing
code_ink        = ["#61afef", "#c678dd", "#98c379", "#e5c07b"]
screen_blocked  = "#4a1d1d"   # waiting on a permission
blocked_ink     = ["#ff6b6b"]
screen_thinking = "#262a36"
thinking_dot    = "#e5c07b"   # the dot that walks across while thinking
screen_reading  = "#e8e8e0"
reading_ink     = ["#8a8a90"]
screen_running  = "#0d120d"   # running a command
running_ink     = ["#7fd962"]
# On the wall.
window_frame   = "#d8dee9"
glass          = "#88c0d0"
glass_shine    = "#b5dbe6"
painting_frame = "#a3815b"
paintings      = ["#81a1c1", "#bf616a"]   # one canvas per painting, in turn
painting_hill  = "#a3be8c"
clock_face     = "#e5e9f0"
clock_hand     = "#bf616a"
# On the floor.
shelf             = "#6b4a2f"
shelf_back        = "#3e2b1c"
books             = ["#bf616a", "#5e81ac", "#a3be8c", "#ebcb8b", "#b48ead"]
machine           = "#5a5f6b"   # the coffee machine
machine_panel     = "#2e3440"
machine_tray      = "#1c1f26"
machine_base      = "#434c5e"
machine_light     = "#bf616a"   # its light blinks between these two
machine_light_off = "#7f2f35"
cooler_bottle     = "#88c0d0"
cooler_body       = "#d8dee9"
cooler_tap        = "#5e81ac"
sofa              = "#7a4b5e"   # its back and arms are drawn darker
sofa_leg          = "#5c4630"
leaf              = "#4caf50"
pot               = "#a0522d"
pot_base          = "#7a3e22"

[pet]
fur    = "#d08770"
shade  = "#a0583a"   # stripes and closed eyes
eye    = "#2e3440"
nose   = "#bf616a"
muzzle = "#eceff4"
bed_rim = "#4c566a"
bed_pad = "#5e81ac"
bowl    = "#d8dee9"
food    = "#ebcb8b"

[ui]
background = "#16181d"   # windows: terminals, board, settings
text       = "#dcdfe4"   # also terminal text without colors of its own
accent     = "#61afef"   # window borders

[custom]
# Colors of your own, for the letters below.
antenna = "#39ff14"

[letters]
# Letters your grid files may use besides the built-in ones.
Z = "custom.antenna"
g = "furniture.glass"

[terminal]
# The 16 ANSI colors programs ask for: black, red, green, yellow, blue,
# magenta, cyan, white, then their bright versions.
palette = ["#06080f", "#cb7c94", "#b7cc85", "#ffe066", "#7fb4ca", "#ff8dd7", "#7aa89f", "#f3f6f9",
           "#8a8fa3", "#de8fa8", "#d1e8a9", "#fff7b1", "#a3d4d5", "#ffaeea", "#7fb4ca", "#f3f6f9"]
```

- Colors are `#rrggbb`. Lists must have at least one color.
- Each agent keeps the same hair, skin, pants and shirt for as long as it
  runs. Changing a list's length can give agents a different look.
- `agents` is more than a shirt: it is the color of the agent's name on the
  floor, in the station strip and in the hub's usage tab.
- `[terminal] palette` takes exactly 16 colors. Without it, terminals keep
  their usual colors (the built-in theme has none). It recolors only those
  16; programs that ask for exact RGB colors or 256-color indexes past 15
  keep them.
- Applying a theme recolors open windows and terminals at once.
- `[custom]` and `[letters]` are explained in
  [Your own colors and letters](#your-own-colors-and-letters).

## Your own colors and letters

Every grid file draws with its own letters, each one standard color. When a
shape needs a color no standard one fits, like an alien's antenna, give it
a name in `[custom]` and a letter in `[letters]`:

```toml
[custom]
antenna = "#39ff14"

[letters]
Z = "custom.antenna"    # one of your [custom] colors
g = "furniture.glass"   # or any standard color, by section and key
```

Now `Z` and `g` work in every grid file of the theme: poses, room tiles,
furniture and the pet. An alien with a green antenna is a pose with a `Z`
on top of its head.

- A `[custom]` key is lowercase letters, digits and `_`; its value is one
  `#rrggbb` color.
- A `[letters]` key is one letter, and not one the built-in grids already
  use, so a built-in letter always means the same thing. The free letters
  are `D I J O Q V X Y Z` and `c d e f g h i j l m n o p q r t u v x y z`;
  `.`, `#` and the digits are taken too.
- A letter's value is a role, `"section.key"`: one of the theme's `[custom]`
  colors as `custom.<key>`, or a standard color like `room.baseboard`. It is
  one color: lists such as `palette.agents` or `furniture.books` are refused.

A palette (see [Wear another theme's colors](#wear-another-themes-colors))
carries its `[custom]` colors too: each one replaces the theme's color of
the same key. The theme keeps its letters and its shapes, and a letter of a
standard color, like `g` above, takes the palette's color for it.

A setting that breaks a rule stops the office at start with an error naming
`theme.toml`, the line and the setting; the error for a taken letter lists
the free ones. A grid file with a letter that is neither its own nor one of
the theme's says which letters it may use, the theme's included.

## Poses

A pose file holds one or two frames. A frame is a grid of 8 columns and 14
rows; frames are separated by one blank line. Each character is one pixel:

| Letter | Pixel |
|---|---|
| `H` | hair |
| `S` | skin |
| `E` | eye |
| `B` | shirt (the agent's color) |
| `b` | shirt in shadow (darker shirt) |
| `P` | pants |
| `F` | shoes |
| `W` | paper |
| `w` | line on the paper |
| `.` | transparent |

| Pose | Frames | When |
|---|---|---|
| `walk-down`, `walk-up`, `walk-side` | 2 | walking; the two frames alternate |
| `stand-down`, `stand-up`, `stand-side` | 1 | standing still |
| `typing` | 2 | at the desk, writing or running a tool |
| `reading` | 2 | at the desk, reading (holds a page) |
| `thinking` | 2 | at the desk, thinking |
| `blocked` | 2 | at the desk, waiting on a permission (hand raised) |
| `asleep` | 2 | at the desk, idle for a while (only the first frame is drawn today) |
| `lounging` | 1 | on the sofa |

Rules the office checks when it starts:

- **Seated poses leave the last two rows empty** (`.`): the desk covers the
  legs.
- Every frame is 8 columns by 14 rows, uses only the letters above and
  the theme's own [letters](#your-own-colors-and-letters), and each pose has its number of frames.

A theme that breaks a rule, or has a bad color, an empty color list, an
unknown setting or section in `theme.toml`, or a `.txt` file in
`characters/` that is not a pose, stops the office at start with an error
naming the file, the field or pose and the line.

How to draw so poses look right (not checked):

- **Side poses face right.** The office mirrors them for walking left.
- **Feet go on the last two rows** of walking and standing poses: that is
  where the office checks for walls and furniture, so feet drawn higher
  look like they float over things.

Two pixels share one terminal cell (the office draws with `▀` and `▄`), so a
person is 8 cells wide and 7 lines tall on screen.

## Room

A room file holds one grid, a tile the office repeats to cover the room.
Each character is one pixel and each letter is one of the `[room]` colors;
there is no `.`, since the room has no holes. The carpets under the areas
are not tiles: they stay rectangles in the `carpets` colors.

**`wall.txt`**: exactly 12 rows (the wall's height) and 1 to 64 columns,
repeated left to right from the room's left edge.

| Letter | Pixel |
|---|---|
| `T` | `wall_top` |
| `F` | `wall_fill` |
| `P` | `wall_panel` |
| `B` | `baseboard` |

The built-in wall is 12×12: a row of `T`, nine rows of one `P` and eleven
`F` (a panel every 12 pixels), then two rows of `B`.

**`floor.txt`**: 1 to 64 columns and 1 to 32 rows, repeated both ways from
the floor's top-left corner, right under the wall.

| Letter | Pixel |
|---|---|
| `A` | `floor_a` |
| `B` | `floor_b` |
| `S` | `floor_seam` |

The built-in floor is 16×8, two planks 4 pixels high whose seams shift half
a plank: four rows of one `S` and fifteen `A`, then four rows of eight `B`,
one `S` and seven `B`.

Rules the office checks when it starts:

- The wall has exactly 12 rows; the floor has 1 to 32.
- Every row of a tile has the same width, 1 to 64 columns.
- A tile uses only its own letters and the theme's own [letters](#your-own-colors-and-letters):
  no `.` and no other letter.

A tile that breaks a rule, or a `.txt` file in `room/` that is not `wall` or
`floor`, stops the office at start with an error naming the file, the line
and what the tile needs.

## Furniture

A furniture file holds one grid, the shape of one piece. Each character is
one pixel, `.` is transparent, and each letter is one of the piece's colors
from `[furniture]` (or `[room]`, where the table says so). Every piece has a
fixed size: a theme changes its shape and colors inside that box, never the
box, so desks, paths and clicks stay where they are.

**`desk.txt`**: 24×13, a station from the monitor down to the desk legs.

| Letter | Pixel |
|---|---|
| `T` | `desk_top` |
| `F` | `desk_front` |
| `L` | `desk_leg` |
| `M` | `bezel` (the monitor's frame) |
| `U` | `mug` |
| `C` | `coffee` |
| `#` | the monitor's screen |

**`chair.txt`**: 10×6.

| Letter | Pixel |
|---|---|
| `B` | `chair_back` |
| `E` | `chair_edge` |

**`window.txt`**: 16×8.

| Letter | Pixel |
|---|---|
| `F` | `window_frame` |
| `G` | `glass` |
| `S` | `glass_shine` |

**`painting.txt`**: 8×6.

| Letter | Pixel |
|---|---|
| `F` | `painting_frame` |
| `C` | the canvas: one of `paintings`, one per painting in turn |
| `H` | `painting_hill` |

**`clock.txt`**: 5×5. The hand is drawn over it.

| Letter | Pixel |
|---|---|
| `F` | `clock_face` |
| `C` | the center, `[room] baseboard` |

**`bookshelf.txt`**: 14×18.

| Letter | Pixel |
|---|---|
| `S` | `shelf` |
| `s` | `shelf_back` |
| `1`–`9` | a book: `1` is the first color of `books`, `2` the second, and so on, starting over when the list runs out |

**`coffee.txt`**: 8×14.

| Letter | Pixel |
|---|---|
| `M` | `machine` |
| `P` | `machine_panel` |
| `T` | `machine_tray` |
| `B` | `machine_base` |
| `U` | `mug` |
| `L` | the light: blinks between `machine_light` and `machine_light_off` |

**`cooler.txt`**: 6×12.

| Letter | Pixel |
|---|---|
| `B` | `cooler_bottle` |
| `C` | `cooler_body` |
| `T` | `cooler_tap` |

**`sofa.txt`**: 20×10.

| Letter | Pixel |
|---|---|
| `S` | `sofa` |
| `s` | `sofa`, a bit darker (the back) |
| `a` | `sofa`, darker still (the arms) |
| `L` | `sofa_leg` |

**`plant.txt`**: 6×10.

| Letter | Pixel |
|---|---|
| `L` | `leaf` |
| `P` | `pot` |
| `B` | `pot_base` |

Rules the office checks when it starts:

- Every piece has exactly its size and uses only its own letters, `.` and
  the theme's own [letters](#your-own-colors-and-letters).
- **The desk's screen is fixed**: `#` fills exactly the 6×5 block at
  columns 3–8 of rows 2–6, where the office shows what the agent does. A `#`
  anywhere else, or a cell of that block that is not `#`, is an error.

A piece that breaks a rule, or a `.txt` file in `furniture/` that is not a
piece, stops the office at start with an error naming the file, the line and
what the piece needs.

How to draw so furniture looks right (not checked):

- **Keep the bottom of floor pieces solid**: agents walk around the part of
  a piece that stands on the floor, which stays the same whatever you draw.
- One frame per piece: only the coffee light, the monitor's screen and the
  clock's hand move.

## Pet

The pet lives in the bottom-left corner of the floor: a bed, the pet on it
and its bowl beside it. Its mood picks the file it is drawn from; each mood
file holds the frames that mood plays. A frame is a grid of 12 columns and 8
rows, frames are separated by one blank line, and each character is one
pixel in a `[pet]` color:

| Letter | Pixel |
|---|---|
| `K` | `fur` |
| `k` | `shade` (stripes, closed eyes) |
| `E` | `eye` |
| `N` | `nose` |
| `W` | `muzzle` |
| `.` | transparent |

A three-eyed cat is a cat with three `E`.

| File | Frames | How they play |
|---|---|---|
| `sleeping` | 2 | nobody works; they alternate every 8 ticks (breathing) |
| `busy` | 3 | agents work; frames 1 and 2 alternate every 4 ticks (a tail wag), and frame 3 shows instead of frame 1 now and then (a blink) |
| `tired` | 2 | a limit is at 70% or more; they alternate every 16 ticks |
| `exhausted` | 2 | a limit is at 90% or more; they alternate every 12 ticks |
| `hungry` | 2 | no task was checked off today; they alternate every 6 ticks (meowing) |
| `alert` | 1 | an agent waits on a permission; it holds still while its `!` blinks |
| `eating` | 2 | a task was just checked off; they alternate every 3 ticks (chewing) |

A tick is one step of the office's animation. The bubbles over the pet (`z`, `~`, `x`, `!`, the fish) are not part of
the theme.

The rest of the corner is two pieces, drawn like furniture:

**`bed.txt`**: 14×4, at the bottom of the corner, under the pet.

| Letter | Pixel |
|---|---|
| `R` | `bed_rim` |
| `P` | `bed_pad` |

**`bowl.txt`**: 4×3, to the right of the bed.

| Letter | Pixel |
|---|---|
| `B` | `bowl` |
| `F` | `food`, drawn only while the pet is not hungry |

Rules the office checks when it starts:

- Every frame is 12 columns by 8 rows and uses only the letters above, `.`
  and the theme's own [letters](#your-own-colors-and-letters); each mood file has exactly its
  number of frames.
- The bed and the bowl have exactly their size and use only their own
  letters, `.` and the theme's own letters.

A pet file that breaks a rule, or a `.txt` file in `pet/` that is not one
of the files above, stops the office at start with an error naming the
file, the line and what it needs: the size, the letters or the number of
frames.
