# wuDict2 Android identity

wuDict2 is an Android fork of [WuWeiDict](https://github.com/wuweidict/wudict).
The upstream copyright notices and GPL-3.0-or-later license remain in place.
The fork's repository on GitHub is [DmShAl/wudict2](https://github.com/DmShAl/wudict2)
(renamed from `DmShAl/wudict` on 2026-09-20; the old URL redirects).
The Android brand and application ID are distinct. The Java package and Gradle
namespace stay `com.legbehindneck.wudict` to reduce merge conflicts with
upstream; a small activity in `com.dmshepeta.wudict2` provides a distinct lookup
component. The Go module path, server API, and internal `wudict` configuration
names remain compatible with upstream.

| Build | Application ID | Launcher name | Lookup activity |
| --- | --- | --- | --- |
| FOSS/Play release | `com.dmshepeta.wudict2` | wuDict2 | `com.dmshepeta.wudict2.LookupActivity` |
| FOSS/Play debug | `com.dmshepeta.wudict2.debug` | wuDict2 Debug | `com.dmshepeta.wudict2.LookupActivity` |

The launcher icon is the fork's own too: upstream's mark with a "2" drawn in it,
so the two apps are told apart on a home screen. The phone's copy is
`android/app/src/main/res/drawable/ic_launcher_foreground.xml` (adaptive and
monochrome), and `tools/make-icons.sh` renders the Play listing's `icon.png` and
`featureGraphic.png` from that same mark with that same digit. The mark itself
stays upstream's `internal/server/web/favicon.svg`, untouched, and so do the
renditions that are only ever a few pixels tall — the tray PNGs, the served
favicon, the `.icns`/`.ico`: a digit at 16px is a smudge, and no desktop build
ships from this fork.

Android uses `https://wudict2.localhost` as its stable WebView origin once
`settings.json` marks migration complete. Before that file exists, it temporarily
uses the previous `http://127.0.0.1:<port>` origin so the in-process Go bridge
can import that WebView profile's localStorage. Android never opens a TCP
listener and does not support server mode. The old port preference is consulted
only for that one-time profile import. Android shell preferences and app-owned
files remain in the wuDict2 app's own storage. The shared HTTP API remains
available to browser and explicit server launch modes on other platforms.

On `dev2`, Android starts the shared Go runtime from the JNI library inside
the app process. The WebView calls Go operations through JNI and intercepts
page, asset, stylesheet, dictionary-resource and ranged-media reads in Go; the
Android runtime opens no HTTP listener. The APK contains only the in-process Go
library; startup reports an error if it cannot load. Runtime changes that need
a fresh registry restart the app process.
The main page's configuration read, preference reads/writes, style reads/writes,
style preset reads/writes and custom style file listing, upload and deletion
now go through the WebView's Java bridge to shared Go operations. The main
dictionary list and main-page search stream through a cancellable Go/JNI bridge;
ordinary GET Rescan folders and Browse's word/find streams use that bridge too.
Browse's nonstream page and chooser also use shared Go operations via JNI.
The main-screen Wildcards list stream, Rebuild status/start/stop and explicit
index-work status/start/stop, the Rescan Update dictionaries progress stream,
single-dictionary ingest progress, Dictionary Groups data and edits, and
Browse/Wildcards result pages use the same bridge.
Browse and dictionary-list timing diagnostics use JNI. The Android page shows
an `A59` stage marker beside the brand in this build. Language preference
saving and scripted-article inline lookups use the direct Go/JNI bridge;
Browse, Edit Folders, Lemmatization and Dictionary Groups pages, embedded
assets and user stylesheets are answered from Go in-process. Edit Folders'
built-in how-to copy and restore actions also call Go directly. The main page
document now also loads from Go through WebView resource interception. `/files/`
and `/res/` requests, including Range requests, are answered through Go resource
interception and a streaming JNI InputStream. A47 returned Go's partial `206`
response directly to WebView, but duplicate `Content-Length` and `Content-Type`
headers caused large-video playback to fail. A48 removed those duplicates and
reports the remaining stream length to WebView. Emulator logs then exposed an
Android WebView bug: it applies the requested Range offset a second time to a
custom `206` response, which makes tail ranges fail. A49 wraps eligible single
ranges so `skip()` accounts for WebView's second offset while reads still start
at the Go-provided range. The user confirmed large-video playback works with
A49. A50 moves Android's System Log export from local HTTP to the Go/JNI bridge;
the user confirmed it works. A51 moves Java archive intake status, URL/path
import, confirmation, cancellation and chunked content-URI upload from local
HTTP to the Go/JNI bridge. Unknown-length providers are streamed without
buffering the archive in memory; the Go spool enforces the existing 16 GiB
limit. A52 sends Android's active/background/restricted power state directly to
Go over JNI, replaying the latest state when the runtime becomes ready;
`/api/power` remains for HTTP clients. The user confirmed the app works on the
emulator; the first minimize/restore observation showed the launcher bringing
the existing activity back, with no app restart. A53 routes the standalone
`/groups` filter editor's groups.ini operations and dictionary-facet scan
through Go/JNI on Android. The browser path keeps HTTP. `build-android.cmd debug
intel` passes; the user confirmed A53 works on the emulator. A54 changes runtime
readiness, launcher Settings' effective-config read, and Exit's busy check to
JNI or local Android state. A55 moves Play SAF post-import rescan to a cancellable
Go/JNI stream. A56 removes Android's TCP listener and routes diagnostic variants
of dictionary listing and folder rescan through the same direct streams. The
FOSS debug APK builds for arm64 and x86_64; Play Java compilation passes.
Emulator verification of A56 is pending. A57 connects Android WebView settings
to the shared file-backed storage through JNI: on first launch it imports old
same-origin localStorage values, then reads and writes `settings.json` and
`history.json` through Go. The user confirmed settings/history migration and
persistence work on the emulator. A58 selects the old origin only while
`settings.json` is absent; after that it uses
`https://wudict2.localhost` without a port. A shared resource adapter serves
pages and ranged media from Go in both the main and lookup WebViews. The FOSS
debug APK builds for arm64 and x86_64; the user confirmed it works on the
emulator. A59 removes listener-specific controls and the FOSS network-listening
implementation from Android. The System screen hides the access-key section,
and launcher Settings no longer offers a port or server restart. The old port
preference remains available only until first-run localStorage import completes.
FOSS and Play debug APKs both build; emulator verification of the removed
settings is pending.
A43 tried a private Android ContentProvider for seekable video, then A44 tried
it for audio. Emulator diagnostics established that WebView rejects `content://`
media from this HTTP page: audio logged "Not allowed to load local resource",
and the large test video's media error was "Media load rejected by URL safety
check". A45 restored audio; A46 restores video and removes the unused provider
and JNI seek bridge. A49 works around Android WebView's repeated Range offset
in the Go/JNI stream; the user confirmed playback works.
Choosing a window background image enables and locks the Color checkbox; choosing
`None` unlocks it again, matching the Windows settings behavior.
The article's `Words`
return button appears left of the arrows in the bottom status bar of the Android
WebView. All Android app pages, API operations and article media now use Go/JNI;
local HTTP remains for browser and explicit server launch modes on other platforms.

From Windows cmd, `build-android.cmd release` makes the FOSS release APK and
`build-android.cmd debug` makes the FOSS debug APK. The second argument
(`original` or `sh`) is retired and produces an error. Gradle's FOSS and Play
tasks still work, including `assemblePlayRelease` and `bundlePlayRelease`.
The APKs are named
`wudict2-android-arm64-v8a-<flavour>[-debug|-unsigned].apk`.
The Play bundle is `wudict2-play-release.aab`. Release builds without a
configured keystore produce an unsigned APK; do not treat that as installable.

`build-android.cmd debug intel` cross-compiles the in-process Go library
for x86_64 as well, and ships both ABIs in one debug APK
(`wudict2-android-arm64-v8a-x86_64-foss-debug.apk`), so it installs on an x86_64
emulator and on arm64 hardware alike. This is not a convenience: the emulator's
ARM translation runs ordinary arm64 binaries but crashes every Go binary, a
hello-world included, so without an x86_64 server there is no way to run the
app on one. The flag is debug-only and the x86_64 library lives in
`android/app/src/emuX86/jniLibs/`, a source directory no variant reads unless
the build passes `-PemuX86=1` — which only the debug source set does — so a
release APK stays arm64 and nothing else. Arm64-v8a emulator images are not an
alternative: on an x86 host they run under full emulation, slower than the
translation they would be replacing.

The old `_sh` application ID and this ID are separate Android apps. Android
does not move preferences, prepared dictionaries, or app-owned files between
them automatically. Do not remove the old installation before independently
copying any data you want to keep. External readers may need to select the new
`LookupActivity` component again. FOSS and Play releases share an application
ID, so they replace rather than coexist with one another when signed compatibly.

The package ID is intended for Google Play, but its availability and the app's
eligibility must be checked in Play Console. The Play flavour targets API 36,
uses SAF rather than all-files access, and has a bundle build task; these source
settings alone do not establish store approval. No signing key is created here.

Keep `master` synchronized only with upstream. Develop on `dev` and branch
from `dev`; review and merge upstream changes into `dev` when desired. Avoid
renaming the Go module or broad internal identifiers solely for Android branding,
so upstream changes stay easier to apply.

Handoff snapshot (2026-09-19): checkout `D:\Projects\Android\wudict` was
clean on `dev` at `b23318a` before this documentation update. Only `origin`
(`DmShAl/wudict`) was configured as a remote; local `master` exists for upstream
sync. Recheck status and remotes before any Git operation. Do not switch or
merge merely to resume this work.

## Where to continue

| Concern | Code |
| --- | --- |
| Android IDs, FOSS/Play flavors, debug suffix, artifact names | `android/app/build.gradle` |
| Exported launcher/lookup activities and shortcut resource | `android/app/src/main/AndroidManifest.xml`; `src/main/res/xml/shortcuts.xml` and `src/debug/res/xml/shortcuts.xml` |
| Distinct external-reader component | `android/app/src/main/java/com/dmshepeta/wudict2/LookupActivity.java`, extending the unchanged upstream-package implementation |
| Launcher icon, Play listing images | `android/app/src/main/res/drawable/ic_launcher_foreground.xml` (the icon the phone shows); `tools/make-icons.sh` renders both Play images from `internal/server/web/favicon.svg` with the same digit |
| Legacy WebView origin for settings import | `android/app/src/main/java/com/legbehindneck/wudict/Shell.java`: `legacyOrigin`, `prepareOrigin`; `ShellPrefs.port()` reads the former SERVER_PORT preference only while `settings.json` is absent |
| In-process Go startup and readiness | `android/app/src/main/java/com/legbehindneck/wudict/ServerProcess.java`: `run`, `runtimeReady`; `AppDirs.java` provides the app-specific library directory |
| In-process resource dispatch | `android/app/src/main/java/com/legbehindneck/wudict/GoAppResource.java`; both Android WebViews use Go/JNI and no server mode is supported |
| Build entry points | `build-android.cmd` (Windows, rebuilds Go and FOSS APK), `build-windows.cmd` (Windows desktop `wuDict2.exe` and Inno Setup installer under `release`; `console` builds the same executable with terminal output), `Makefile`, `.github/workflows/build-android.yml`, `fastlane/Appfile` |

The old Android build used a loopback listener. The current build starts Go
inside the app process and opens no TCP socket. A58 uses the former URL only
when `settings.json` is absent, so Go can read the old WebView localStorage;
after migration, every app page and resource uses the stable secure origin.

## Build and verification record

Run in Windows cmd from the repository root:

```bat
build-android.cmd release
build-android.cmd debug
```

The script checks Go, Git, Java, SDK, and NDK; `adb` is optional. It reads an
optional ignored `build-android.local.bat` for local settings. Do not read or
print signing secrets. Without a configured release keystore, the release APK
is named `-unsigned` and must be signed before installation. Direct Gradle
tasks do not rebuild the embedded Go binary. For Play, rebuild Go first, then
use `android\gradlew.bat :app:assemblePlayDebug` or
`android\gradlew.bat :app:bundlePlayRelease` as appropriate.

At the last check on 2026-09-19, `build-android.cmd release` rebuilt the Go
binary and FOSS release APK successfully; `assembleFossDebug` and
`assemblePlayDebug` also passed after the port change. The release APK's
signature verified locally. The APK is under
`android/app/build/outputs/apk/foss/release/`. A Play AAB exists under
`android/app/build/outputs/bundle/playRelease/`, but it predates the port fix
and needs rebuilding before use. `git diff --check` passed for the port edit.
For artifact identity, inspect the merged manifest or APK with SDK `aapt2`:
release package `com.dmshepeta.wudict2`, upstream-package `MainActivity`, and
fork-package exported `LookupActivity`; inspect the debug shortcut target
package separately. These source and artifact checks do not establish device
behavior.

On 2026-10-17, `build-android.cmd debug intel` built the FOSS APK for arm64 and
x86_64. Gradle assembled both `:app:assembleFossDebug` and
`:app:assemblePlayDebug` after the Android server controls were removed. The
user has not yet verified the A59 settings screen on the emulator.

## Unfinished work

- Install the current wuDict2 release on a device and test simultaneous wuDict
  and wuDict2 operation in both launch orders. Confirm each displays its own
  library and that closing one does not stop the other. No device test has been
  run for the port fix.
- Test external-reader selection of
  `com.dmshepeta.wudict2.LookupActivity`, including the case where both apps
  are installed. The component exists in the built manifest, but reader choice
  has not been exercised.
- Rebuild and inspect the Play release AAB after the port change if Play work
  resumes. Publication, package-name availability, and store eligibility
  require separate Play Console checks; nothing was uploaded.

## Unverified assumptions

- `/api/config.libDir` is expected to resolve to the same canonical path as
  `AppDirs.dbDir` for the app's Go runtime. Source inspection supports this;
  device startup timing has not been independently checked in this session.
- The A59 emulator check should confirm the Android System screen has no access
  or listener settings and the launcher Settings screen has no port or restart
  controls. Both FOSS and Play debug builds compile successfully.
