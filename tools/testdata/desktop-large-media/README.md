# Large media streaming fixture

This directory contains a DSL source dictionary and a ready-to-open StarDict fixture for checking large image and video resources in wuDict2.

## Source DSL

Open `desktop-large-media.dsl` in wuDict2. Its adjacent resource directory is `desktop-large-media.dsl.files` and contains:

- `large-image.png` — about 53 MiB.
- `as.mp4` — about 189 MiB.
- `large-image.jpg` — a smaller alternate image.

The DSL article references the PNG and MP4. These are the actual filenames; keep them unchanged unless you also update the DSL.

## StarDict

Add `stardict/desktop-large-media.ifo` to wuDict2. Its `res/` directory contains copies of `large-image.png` and `as.mp4`. The `large-media-test` entry displays the image and embeds a controllable video.

The StarDict fixture is self-contained. The two media files are copied from the DSL resource folder.
