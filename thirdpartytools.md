# Third-party build tools

Machine-local tools used by DictShelf build and asset scripts. This is the one
place that says **what each tool is called here**; the recipes that use them are
in `docs/WINDOWS-VERIFY.md`, and the index that leads to both is `docs/INDEX.md`.

- Inkscape: `C:\Tools\inkscape\bin\inkscape.exe` — export SVG artwork to PNG assets, including tray icons.
- Inno Setup 7: `D:\ProgSoft\InnoSetup7\ISCC.exe` — compile Windows installer. The Inno Setup **5** also on this machine (`D:\ProgSoft\InnoSetup5\`) cannot read our script at all; never point `ISCC_PATH` at it.
- MinGW GCC 13.1.0: `C:\Qt\Tools\mingw1310_64\bin\gcc.exe` — Windows cgo build compiler.
- GNU Make 4.2.1: `C:\Qt\Tools\mingw1310_64\bin\mingw32-make.exe` — the Makefile targets (`make build`, `make test`, `make vet`, `make check`, `make help`).
  **There is no `make` on PATH here**, so `make check` typed as such answers "command not found" and reads as "this machine cannot run the Makefile"; it can. Call it by its real name — `/c/Qt/Tools/mingw1310_64/bin/mingw32-make.exe help` — or add that directory to PATH. The recipes are POSIX `sh`: run it from Git Bash (GNU Make finds `sh` there), not from `cmd`.
- Node.js v24.21.0 with npm: `C:\Program Files\nodejs` — installed and on PATH (earlier notes said this machine had no `node`; that is out of date). Nothing in the build needs it: it is for the optional JS checks (`make i18n-check-js`, `node tools/i18n-test.cjs`) and for `redocly` behind `make api-lint`.
- Playwright: `/d/Temp/pw` (`NODE_PATH=/d/Temp/pw/node_modules`, launch with `channel: 'msedge'`) — browser checks of the served page. Outside the repository on purpose: it is a check-only tool, not a dependency.
- Android SDK: `C:\Users\shepe\AppData\Local\Android\Sdk` (`adb`, `emulator`, `apksigner`, `aapt2`, `build-tools`). Its AVDs are x86_64, which is why emulator builds pass `intel`.
- Python 3: on PATH as `python3` — used for one-off checks (counting strings in a binary, since this shell's `grep` is `ugrep`).

## Where the values live

The paths a build script needs are set in two **ignored** local files, which is
why they are not in git:

| File | Variables |
| --- | --- |
| `build-windows.local.bat` | `GCC_PATH`, `ISCC_PATH` |
| `build-android.local.bat` | `ANDROID_HOME`, `JAVA_HOME` (commented-out examples), and the signing material: `KEYSTORE`, `KEY_ALIAS`, `STORE_PASSWORD`, `KEY_PASSWORD` |

Both are read automatically when present (`build-windows.cmd`, `build-android.cmd`),
and both are covered by `.gitignore`. **Never read or print the signing values**;
the tool paths are safe to read and are quoted above only because finding them is
the point of this file.

These paths are specific to the maintainer's Windows machine. Build scripts should allow environment overrides where supported; do not copy these third-party tools into the repository.
