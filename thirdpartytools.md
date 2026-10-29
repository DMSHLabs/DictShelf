# Third-party build tools

Machine-local tools used by DictShelf build and asset scripts:

- Inkscape: `C:\Tools\inkscape\bin\inkscape.exe` — export SVG artwork to PNG assets, including tray icons.
- Inno Setup: `D:\ProgSoft\InnoSetup7\ISCC.exe` — compile Windows installer.
- MinGW GCC: `C:\Qt\Tools\mingw1310_64\bin\gcc.exe` — Windows cgo build compiler.

These paths are specific to the maintainer's Windows machine. Build scripts should allow environment overrides where supported; do not copy these third-party tools into the repository.
