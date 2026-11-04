# Dictzip v2, and the DSL reader that must accept both formats

Written down for the two places these rules bind: the separate Windows project
`D:\Projects\Android\dictzip-win64`, and this repository's wudict DSL reader.
Nothing here is optional - a format change made for convenience is a change that
breaks readers, which is exactly what v2 exists to avoid.

- `D:\Projects\Android\dictzip-win64` is its own project. Its
  `build-windows.local.bat` carries the configured GCC path; inspect **only** the
  `GCC_PATH` setting when locating the compiler (the file also holds private
  settings).
- The utility keeps **two explicit output formats**: v1 is the existing format
  and stays the default unless the user selects otherwise; v2 is a distinct,
  intentionally incompatible format. **Old readers are not required to recognize
  v2** - that is the point of a separate number, not a defect to fix.
- **V2 has a private `DZV2` preamble** with a 64-bit uncompressed length and a
  32-bit per-block compressed-length table; the compressed payload follows.
- **Keep v1's block size exactly unchanged in v2.** Do not enlarge blocks to fit
  an index into the header, and do not adapt v2 to gzip header limits: both are
  the same mistake of letting the container dictate the data.
- **The wudict DSL reader must accept both v1 and v2.** Keep v1 behaviour intact
  and verify both formats with generated fixtures and the Windows GCC build - not
  by reading the writer's code.
