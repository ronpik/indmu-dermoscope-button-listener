# Building and maintaining the installer

For a developer touching [`helper.iss`](helper.iss), the Inno Setup 6 script that packages
`helper.exe` into a per-user Windows installer. If you just want to *use* the installer, see
[`../README.md`](../README.md#install) or [`../CLIENT-HANDOFF.md`](../CLIENT-HANDOFF.md).

---

## Building locally

Two prerequisites, one for each half of the pipeline:

| Tool | For | Install |
|---|---|---|
| MSYS2 mingw-w64 toolchain | Building `helper.exe` itself | `pacman -S --needed mingw-w64-x86_64-gcc mingw-w64-x86_64-binutils make` (see [`../README.md`](../README.md#build)) |
| Inno Setup 6 (`ISCC.exe`) | Compiling this `.iss` script into a setup exe | `winget install JRSoftware.InnoSetup` |

Then, from `windows-helper/` in an MSYS2 MINGW64 shell:

```bash
make static VERSION=1.2.3
make installer VERSION=1.2.3
```

`make installer` depends on `static`, so the first line above is technically redundant — running
just `make installer VERSION=1.2.3` rebuilds `dist-static/helper.exe` for that same version
automatically. **The installer only ever packages `dist-static/helper.exe`** (see the `[Files]`
section below); there is no path to a `dist/` shared-build install, and none is planned — the
static build's whole reason to exist is "one file, no DLLs to go missing," which is exactly what
you want wrapped in an installer too.

`VERSION` is a required `/D` define, not a make convenience — see "VERSION is required" below.

If `ISCC` isn't on `PATH` and isn't at Inno Setup 6's default install location
(`C:/Program Files (x86)/Inno Setup 6/ISCC.exe`), point `make` at it explicitly:

```bash
make installer VERSION=1.2.3 ISCC="D:/Tools/Inno Setup 6/ISCC.exe"
```

`make check-iscc` (a dependency of `installer`) prints that same install/override guidance if it
can't find `ISCC` at all.

**`winget install JRSoftware.InnoSetup` without an elevated shell installs per-user**, landing at
`%LOCALAPPDATA%\Programs\Inno Setup 6\ISCC.exe` instead of the `Program Files (x86)` default above
— which is exactly what `make installer`'s bare `command -v iscc` / default-path check won't find.
If `make check-iscc` fails right after a fresh `winget install`, either re-run the install from an
elevated prompt (`winget install JRSoftware.InnoSetup` as Administrator installs to
`Program Files (x86)` machine-wide), or just point `make` at the per-user path:

```bash
make installer VERSION=1.2.3 ISCC="C:/Users/<you>/AppData/Local/Programs/Inno Setup 6/ISCC.exe"
```

### The raw ISCC command line

Useful for debugging `helper.iss` directly without going through `make` — e.g. to see ISCC's own
error output unfiltered, or to iterate on the script without waiting for a full `static` rebuild
each time (as long as `dist-static/helper.exe` already exists from a previous build):

```bash
# from windows-helper/installer/
"C:/Program Files (x86)/Inno Setup 6/ISCC.exe" /DVERSION=1.2.3 helper.iss
```

Output lands in `windows-helper/dist-installer/DermoscopeHelper-Setup-1.2.3.exe` either way —
`helper.iss`'s own `OutputDir=..\dist-installer` is relative to the script, not to your shell's
cwd.

---

## How `helper.iss` is structured

Section by section, in the order they appear in the file:

| Section | What it's doing |
|---|---|
| Preprocessor block (top) | The `#ifndef VERSION` guard, and the `VersionRaw` → `VersionNoPre` → `VersionNoMeta` → `VersionNumeric` chain that strips a `-rc1`/`+build5` suffix off `VERSION` for the two `VersionInfo*` keys, which — like `FILEVERSION` in [`../helper.rc`](../helper.rc) — only accept numeric fields. `AppVersion` and the output filename keep the full, unstripped `VERSION` string. |
| `[Setup]` | Everything about *how* Setup installs: install location, privilege level, the app identity (`AppId`), the icon, compression, and `AppMutex`. See "Decisions and why" below for the ones worth understanding before you touch them. |
| `[Languages]` | English only — `compiler:Default.isl`. No localization is planned; add a `Name`/`MessagesFile` line here if that ever changes. |
| `[Tasks]` | The two checkboxes on the wizard's "Additional shortcuts" page: `desktopicon` (unchecked by default) and `startupicon` (checked by default — no `Flags: unchecked` line, since checked is Inno's default when the key is simply absent). |
| `[Files]` | One line: `dist-static\helper.exe` → `{app}`. Nothing else ships. |
| `[Icons]` | The four shortcuts: Start Menu entry, its Start Menu uninstall entry, the optional desktop shortcut (`Tasks: desktopicon`), and the Startup-folder shortcut (`Tasks: startupicon`). None sets `IconFilename` — they all inherit the icon straight from `helper.exe`'s own resource 101, so there's nothing here to keep in sync if the icon file ever changes. |
| `[Run]` | The "Launch Dermoscope Helper" postinstall checkbox — `nowait skipifsilent` so it doesn't block Setup's exit and doesn't try to launch anything during a silent/scripted install. |
| `[UninstallDelete]` | `helper.log` and `helper.log.1` — the two files the *running app* creates that Setup's own uninstall log doesn't know about (see "Decisions and why" for why nothing else needs listing here). |

There is no `[Code]` section — nothing here needs Pascal Script. If you're tempted to add one
(a custom wizard page, a runtime check), think first about whether a plain `[Tasks]`/`[Run]`
entry can do it instead; `[Code]` is a lot more code to keep correct.

---

## Decisions and why

### Per-user, `PrivilegesRequired=lowest`, installs under `%LOCALAPPDATA%`

The helper writes `helper.log` (rotated to `helper.log.1`) **next to its own exe** — that's
existing, unchangeable runtime behaviour (see `open_log_file()` in [`../helper.cpp`](../helper.cpp)).
A machine-wide install under `Program Files` is read-only to a non-admin process, which would
silently break logging for anyone without admin rights — and clinic workstations often have none.
Installing per-user under `{localappdata}\Programs\Dermoscope Helper` keeps the exe (and the log
beside it) writable by the same account that runs the tray app, with no elevation prompt.
`PrivilegesRequired=lowest` is the other half of this: it stops Setup itself from asking for admin
rights it doesn't need.

**If you ever "fix" this to a Program Files/admin install:** you'd also need to relocate the log
(e.g. to `%LOCALAPPDATA%` regardless of install dir) or logging silently breaks for non-admin
users. Don't change one without the other.

### The fixed `AppId` GUID

`{59AC4363-9924-4988-95E5-3C011DB8F1AC}` is what lets a newer `DermoscopeHelper-Setup-X.Y.Z.exe`
detect and upgrade an existing install in place, instead of installing side-by-side as a second,
unrelated app with its own Start Menu entry and its own uninstaller. **Never regenerate this
GUID** — doing so breaks in-place upgrades for every machine that already has an older version
installed; their next Setup run would look like a fresh install of something new rather than an
upgrade.

### `AppMutex` — the single most confusing line in this file

```
AppMutex=DermoscopeHelperSingleInstance
```

The helper's actual mutex, created in [`../helper.cpp`](../helper.cpp), is named
`Local\DermoscopeHelperSingleInstance` (`SINGLE_INSTANCE_MUTEX_NAME`). This line has **no**
`Local\` prefix, and that's deliberate, not a bug: `Local\` is simply the *default* namespace a
bare `CreateMutexW` name already resolves into for a non-`Global\` mutex, so
`Local\DermoscopeHelperSingleInstance` and the unprefixed `DermoscopeHelperSingleInstance` name
the exact same kernel object in the current session. Inno's `AppMutex` checks for the unprefixed
name (and separately for a `Global\` one) — it does not understand an explicit `Local\` prefix as
"the same thing." Writing `AppMutex=Local\DermoscopeHelperSingleInstance` here would make Inno
look for a mutex literally named `Local\Local\DermoscopeHelperSingleInstance`, which never
exists, and Setup/Uninstall would stop detecting a running helper at all.

**If you ever rename the mutex in `helper.cpp`:** change the string *after* `Local\` in both
places — `SINGLE_INSTANCE_MUTEX_NAME` in [`../helper.cpp`](../helper.cpp) and the `AppMutex` value
here — and keep the "no `Local\` here" rule in mind. The two can silently drift apart because
nothing else checks that they match; `helper.cpp`'s own comment on `SINGLE_INSTANCE_MUTEX_NAME`
points back at this file for the same reason.

### Startup-folder shortcut, not a registry Run key

The `startupicon` task (the `{userstartup}` shortcut in `[Icons]`) drops a `.lnk` file in the
Startup folder rather than writing an
`HKCU\...\Run` value. A folder shortcut is visible and individually removable by the user via
**Task Manager → Startup apps** (or `shell:startup`) without touching the registry, and it's
tracked by Setup like any other installed file — so it's cleanly removed on uninstall with no
leftover registry value to hunt down if uninstall is ever skipped, or the app folder is deleted by
hand instead of properly uninstalled.

### `VERSION` is a required `/D` define, with no default

```
#ifndef VERSION
  #error VERSION is not defined -- ...
#endif
```

A silently-defaulted `"0.0.0"` setup exe is worse than a build that refuses to produce one at all
— it's easy to publish an unversioned installer by accident if a missing `VERSION` just quietly
falls back to something. `make installer` always passes one (see `VER_CLEAN` in
[`../Makefile`](../Makefile)), so this only bites you if you invoke `ISCC` directly and forget
`/DVERSION=...`.

### Windows Defender flags the exe on execution (`Trojan:Win32/Bearfoos.A!ml`)

This is a real thing you'll hit while testing locally, not a hypothetical. Windows Defender's
real-time protection can flag `dist-static/helper.exe` — and the copy the installer places under
`%LOCALAPPDATA%\Programs\Dermoscope Helper` — as `Trojan:Win32/Bearfoos.A!ml` the moment it
*runs*, and **delete the file**. It's a well-documented machine-learning false positive that
disproportionately hits statically-linked mingw-w64 binaries (`-static -static-libgcc
-static-libstdc++`, exactly what `make static` produces); the file sitting at rest on disk isn't
touched, only an executed copy gets pulled. It'll happen again on the very next build unless you
exclude the relevant folders. Check **Windows Security → Protection history** if a build you just
ran vanishes.

Before doing any local install/uninstall testing, add exclusions (elevated PowerShell):

```powershell
Add-MpPreference -ExclusionPath "C:\path\to\this\repo\windows-helper"
Add-MpPreference -ExclusionPath "$env:LOCALAPPDATA\Programs\Dermoscope Helper"
```

or via the GUI: **Windows Security → Virus & threat protection → Manage settings → Exclusions →
Add an exclusion → Folder**, once for the repo's `windows-helper/` directory (covers
`dist-static/`) and once for the installer's target folder. See
[`../CLIENT-HANDOFF.md`](../CLIENT-HANDOFF.md) for the same instructions phrased for a client
machine. Code signing (see [Code signing](../README.md#code-signing)) is expected to make this
false positive go away entirely, the same way it resolves SmartScreen — this whole section becomes
unnecessary once that's wired up.

---

## How this is built in CI

[`../../.github/workflows/release.yml`](../../.github/workflows/release.yml) builds the installer
as part of every release, right after `make static`: it resolves `ISCC.exe` (installing Inno Setup
via `choco` if the `windows-latest` runner doesn't already have it), then runs
`make installer VERSION=<version> ISCC=<resolved path>`. The installer is an **additional** release
asset alongside the existing bare `helper.exe` and its zip — never a replacement for either; all
three ship on every published release.

---

## Gotchas / when you change things

| If you change... | ...then also |
|---|---|
| `SINGLE_INSTANCE_MUTEX_NAME` in `helper.cpp` | Update `AppMutex` here to the same string after `Local\` (see "AppMutex" above). |
| `AppId` | Don't, unless you deliberately want to break in-place upgrades for every existing install (see "The fixed AppId GUID" above). |
| Resource ID of the app icon in `helper.rc` | Update if any `[Icons]` entry ever grows an explicit `IconFilename`/`IconIndex` — today none do, since they all inherit resource 101 automatically. |
| What files the app writes under `{app}` at runtime | Add them to `[UninstallDelete]`, the same way `helper.log`/`helper.log.1` are listed — Setup's own uninstall log only knows about files *it* installed, not ones the running app creates later. |
| `OutputDir` or `OutputBaseFilename` in `[Setup]` | Update `DIST_INSTALLER` and the success-message path in the `installer` target of [`../Makefile`](../Makefile) to match — they assume today's `..\dist-installer` / `DermoscopeHelper-Setup-{#VERSION}` exactly. |

**If you invoke `ISCC.exe` directly from an MSYS2/Git-Bash shell** (bypassing `make installer`,
e.g. while debugging the raw command above) and get `"You may not specify more than one script
filename."` from ISCC even though your command line looks right: MSYS's runtime auto-converts
argv entries that look like POSIX paths before handing them to a native (non-MSYS) executable, and
a bare leading `/`, exactly what `/DVERSION=...` starts with, is what that heuristic keys off —
so it silently mangles the flag before ISCC ever sees it. `make installer`'s own recipe already
sets `MSYS2_ARG_CONV_EXCL='*'` for this exact reason; if you're calling ISCC by hand, prefix your
command with the same thing: `MSYS2_ARG_CONV_EXCL='*' "ISCC.exe" /DVERSION=1.2.3 helper.iss`.
