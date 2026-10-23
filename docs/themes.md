# Themes

A theme changes the office's colors, how its people look, and the colors of
the windows and terminals. It is a folder
of plain text files on your machine: nothing is downloaded, and you can share
a theme by copying its folder.

It covers the palettes, the colors of the room, the furniture and the pet,
the characters' poses, the windows (terminals, board, settings) and the
terminals' 16 ANSI colors. The shapes of the furniture and the pet are not
themable yet.

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

## Start from the built-in theme

In the office, `s` opens the hub's settings tab: pick a theme and press `enter`
to apply and save it, or press `n` to copy the active theme under a new name.
From a terminal:

```sh
bah theme init my-theme
```

This writes the built-in theme, every color and every pose, to the themes
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

## The folder

```
my-theme/
  theme.toml
  characters/
    walk-down.txt  walk-up.txt  walk-side.txt
    stand-down.txt stand-up.txt stand-side.txt
    typing.txt     reading.txt  thinking.txt
    blocked.txt    asleep.txt   lounging.txt
```

`theme.toml` is required; everything in it and every pose file is optional.
Whatever a theme leaves out, a color or a whole pose, comes from the built-in
theme, so a theme can be a `theme.toml` with one color. Only the pose files
listed above may sit in `characters/` (other `.txt` files are an error; files
with other extensions are ignored).

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
- Every frame is 8 columns by 14 rows, uses only the letters above, and each
  pose has its number of frames.

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
