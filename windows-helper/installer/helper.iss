; Inno Setup 6 script for the Dermoscope Helper installer.
;
; Builds a single per-user setup exe that installs the static helper.exe
; build (windows-helper\dist-static\helper.exe -- built by
; `make static VERSION=X.Y.Z` in windows-helper\Makefile) into the current
; user's LOCALAPPDATA, with a Start Menu shortcut, an optional desktop
; shortcut, and a "run at sign-in" shortcut in the Startup folder.
;
; Build it with:
;     make installer VERSION=1.2.3
; which shells out to (see the ISCC make variable in windows-helper\Makefile):
;     ISCC.exe /DVERSION=1.2.3 windows-helper\installer\helper.iss
; or invoke ISCC directly the same way if you don't have `make`:
;     "C:\Program Files (x86)\Inno Setup 6\ISCC.exe" /DVERSION=1.2.3 helper.iss
; (run from this directory, or give ISCC the full path to this .iss file).
; VERSION is required -- see the #ifndef guard below -- there is no default,
; because a silently-defaulted "0.0.0" setup exe is worse than a build that
; refuses to produce one.
;
; Output lands in windows-helper\dist-installer\, matching the Makefile's
; other build-output layout (dist\, dist-static\, dist-installer\).
;
; ---- Why per-user / LOCALAPPDATA, no admin -------------------------------
; The helper writes its own log file (helper.log, rotated to helper.log.1)
; NEXT TO THE EXE -- that is existing, unchangeable behaviour (see helper.cpp
; open_log_file()). A machine-wide install under Program Files is read-only
; to a non-admin process, which would break logging outright for anyone who
; isn't an administrator. Installing per-user under
; {localappdata}\Programs\Dermoscope Helper keeps the exe (and therefore the
; log next to it) writable by the same account that runs the tray app, with
; no elevation prompt and no UAC dialog -- appropriate for a tray utility
; nobody else on a shared machine needs to see. PrivilegesRequired=lowest
; below is the other half of this: it stops Setup from asking for admin
; rights it does not need and would not usefully use.
;
; ---- Why a Startup-folder shortcut instead of a registry Run key --------
; A shortcut in {userstartup} is visible and individually removable by the
; user via Task Manager -> Startup apps (or shell:startup), and it vanishes
; cleanly on uninstall because it's just a file Setup tracks like any other
; -- no leftover registry value to hunt down if uninstall is ever skipped or
; the app is dragged to the Recycle Bin by hand instead.
;
; ---- AppId --------------------------------------------------------------
; Fixed GUID, do not regenerate: {59AC4363-9924-4988-95E5-3C011DB8F1AC}.
; This is what lets a future installer version detect and upgrade this one
; instead of installing side-by-side.

#ifndef VERSION
  #error VERSION is not defined -- build via "make installer VERSION=X.Y.Z" (or pass /DVERSION=X.Y.Z to ISCC directly); there is no default, so a bare ISCC run fails loudly instead of quietly producing a "DermoscopeHelper-Setup-.exe" with no version in it.
#endif

; ---- Numeric version for VersionInfoVersion / VersionInfoProductVersion --
; AppVersion and the output filename are free-form text and happily display
; a full semver string including a pre-release suffix (e.g. "1.4.2-rc1").
; VersionInfoVersion/VersionInfoProductVersion are stricter: Inno stores them
; in the compiled setup exe's Win32 VERSIONINFO block, which -- like the
; FILEVERSION field in helper.rc -- only has room for numeric WORD fields.
; A literal "-rc1" there would not parse as a number and would fail the
; build, so we derive a cleaned, numeric-only value from VERSION here at
; compile time (mirroring the same "strip the suffix, keep the digits" trick
; windows-helper\Makefile already does for FILEVERSION/PRODUCTVERSION):
;   "1.4.2"      -> "1.4.2.0"
;   "1.4.2-rc1"  -> "1.4.2.0"   (suffix dropped)
;   "1.4.2+build5" -> "1.4.2.0" (build metadata dropped)
; This project's convention (see the Makefile) is a plain X.Y.Z tag with an
; optional -suffix/+meta, so we only strip at the first "-" or "+" rather
; than trying to parse arbitrary version grammars.
; Each stage below defines a fresh name rather than redefining the same one
; in place, so there is no dependence on how (or whether) the preprocessor
; re-expands a macro name inside its own replacement text.
#define VersionRaw VERSION
#if Pos("-", VersionRaw) > 0
  #define VersionNoPre Copy(VersionRaw, 1, Pos("-", VersionRaw) - 1)
#else
  #define VersionNoPre VersionRaw
#endif
#if Pos("+", VersionNoPre) > 0
  #define VersionNoMeta Copy(VersionNoPre, 1, Pos("+", VersionNoPre) - 1)
#else
  #define VersionNoMeta VersionNoPre
#endif
#define VersionNumeric VersionNoMeta + ".0"

[Setup]
AppId={{59AC4363-9924-4988-95E5-3C011DB8F1AC}
AppName=Dermoscope Helper
AppVersion={#VERSION}
AppPublisher=Indmu
VersionInfoVersion={#VersionNumeric}
VersionInfoProductVersion={#VersionNumeric}
DefaultDirName={localappdata}\Programs\Dermoscope Helper
DefaultGroupName=Dermoscope Helper
PrivilegesRequired=lowest
DisableProgramGroupPage=yes
; DisableDirPage=auto: there is exactly one correct install location for a
; per-user app (no admin/machine-wide choice to make, unlike a typical
; Program Files installer), so there is nothing useful for the user to pick.
; "auto" is the documented idiomatic spelling for "don't bother the user with
; this page" -- unlike a hardcoded "yes", it still lets Setup show the page
; in the one case where DefaultDirName can't be resolved up front (it uses no
; {param} constants here, so in practice this behaves exactly like "yes"
; today, but stays correct if that ever changes).
DisableDirPage=auto
; The static build is an x86_64 mingw-w64 binary with no 32-bit fallback, so
; refuse to run/install on anything but 64-bit Windows rather than silently
; doing a 32-bit-mode install that would then fail to launch the exe.
; "x64compatible" (not the older "x64", deprecated since Inno Setup 6.3) is
; the identifier for "any Windows that can run an x64 binary", which includes
; x64 emulation on ARM64 -- exactly the plain x86_64 exe this installer
; carries, with no reason to exclude ARM64-via-emulation machines.
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern
SetupIconFile=..\assets\helper.ico
UninstallDisplayIcon={app}\helper.exe
UninstallDisplayName=Dermoscope Helper
; The helper enforces single-instance itself via a named mutex (see
; SINGLE_INSTANCE_MUTEX_NAME in helper.cpp). Naming it here too lets Setup
; and Uninstall detect a running helper and prompt the user to close it
; before proceeding, instead of installing/removing files out from under a
; live process.
;
; The mutex is created as "Local\DermoscopeHelperSingleInstance". Inno's
; AppMutex checks for the UNPREFIXED name in the current session's namespace
; and also checks "Global\<name>" -- an unprefixed name and "Local\<name>"
; both refer to the SAME kernel object for a session-local (non-Global)
; mutex, since "Local\" is simply the default namespace a bare CreateMutex
; name resolves into. So "AppMutex=DermoscopeHelperSingleInstance" (no
; "Local\" prefix) is what actually matches helper.exe's mutex -- adding
; "Local\" here ourselves would make Inno look for a mutex literally named
; "Local\Local\DermoscopeHelperSingleInstance", which would never exist.
; This looks like it should say "Local\..." to match helper.cpp at a glance;
; it deliberately doesn't. Keep this in sync with
; SINGLE_INSTANCE_MUTEX_NAME in windows-helper\helper.cpp if that ever
; changes -- the string after "Local\" must always be identical here.
AppMutex=DermoscopeHelperSingleInstance
OutputDir=..\dist-installer
OutputBaseFilename=DermoscopeHelper-Setup-{#VERSION}

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "Create a &desktop shortcut"; GroupDescription: "Additional shortcuts:"; Flags: unchecked
Name: "startupicon"; Description: "Start Dermoscope Helper when I sign in"; GroupDescription: "Additional shortcuts:"

[Files]
; Static build only -- deliberately not the shared (dist\) build. It has no
; mingw runtime DLLs to go missing or drift out of sync with the exe, so
; there is nothing else to list here: one file, one Source line.
Source: "..\dist-static\helper.exe"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
; No IconFilename overrides below: every shortcut here points straight at
; helper.exe, which already embeds the app icon as resource 101 (IDI_APP,
; see helper.rc) -- Explorer and the Start Menu pick that up on their own,
; so a redundant IconFilename would just be one more place to update if the
; icon path ever moved.
Name: "{group}\Dermoscope Helper"; Filename: "{app}\helper.exe"
Name: "{group}\Uninstall Dermoscope Helper"; Filename: "{uninstallexe}"
Name: "{autodesktop}\Dermoscope Helper"; Filename: "{app}\helper.exe"; Tasks: desktopicon
; A Startup FOLDER shortcut, not a registry Run key -- see the header
; comment above for why.
Name: "{userstartup}\Dermoscope Helper"; Filename: "{app}\helper.exe"; Tasks: startupicon

[Run]
Filename: "{app}\helper.exe"; Description: "Launch Dermoscope Helper"; Flags: postinstall nowait skipifsilent

[UninstallDelete]
; The helper writes its log next to the exe (see the header comment above);
; Setup's own uninstall log only knows about files IT installed, so the
; runtime-created log files need to be listed explicitly or they'd be left
; behind. Nothing else the app touches lives under {app}, so once these two
; files and helper.exe (auto-removed by Setup) are gone, {app} is empty and
; Setup's uninstaller removes the now-empty directory on its own -- no
; separate "dirifempty" entry is needed.
Type: files; Name: "{app}\helper.log"
Type: files; Name: "{app}\helper.log.1"
