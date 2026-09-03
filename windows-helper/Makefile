# Windows-only dermoscope helper.
# Built with mingw-w64 GCC (install via MSYS2: pacman -S mingw-w64-x86_64-gcc).
#
# Two build modes:
#   make shared   -> dist/helper.exe + runtime DLLs in dist/ (small exe, 3 DLLs alongside)
#   make static   -> dist-static/helper.exe  (single fat exe, no external DLLs)
#   make all      -> both of the above (default)
#   make clean    -> remove both dist dirs, dist-installer/ and build/
#
# Installer (per-user, no admin required):
#   make installer VERSION=1.2.3   builds the static exe, then packages it with
#                                   Inno Setup into
#                                   dist-installer/DermoscopeHelper-Setup-1.2.3.exe
#   Needs ISCC (the Inno Setup 6 command-line compiler) — see the ISCC variable
#   and the check-iscc target below for how it's located and what to do if it
#   isn't installed.
#
# Versioning:
#   make static VERSION=1.2.3   stamps the exe with that version + the app icon
#   VERSION defaults to 0.0.0. Release tags are vX.Y.Z; pass the tag WITHOUT
#   the leading "v". A pre-release suffix (1.2.3-rc1) is fine — it is kept in
#   the displayed version string and stripped from the numeric fields.
#   `make installer` stamps and names the Setup exe with this same cleaned
#   version (see VER_CLEAN), so the two can never disagree.
#
# Output layouts:
#   dist/
#     helper.exe                 (~870 KB, links against mingw runtime)
#     libstdc++-6.dll
#     libgcc_s_seh-1.dll
#     libwinpthread-1.dll
#
#   dist-static/
#     helper.exe                 (~3.4 MB, no external deps — drop anywhere)
#
#   dist-installer/
#     DermoscopeHelper-Setup-X.Y.Z.exe   (per-user Inno Setup installer, wraps
#                                          the dist-static/ exe above)

CXX       ?= g++
CXXFLAGS  ?= -O2 -Wall -std=c++17
SRC       := helper.cpp
LIBS      := -lstrmiids -lole32 -loleaut32 -luuid -lws2_32

# --- Version stamping (icon + VERSIONINFO via windres) ---
# Override on the command line: make static VERSION=1.2.3
VERSION   ?= 0.0.0

# Tolerate a leading "v" (tag name) and a pre-release / build-metadata suffix
# (1.2.3-rc1, 1.2.3+abc): the numeric FILEVERSION fields only ever see digits,
# and missing components fall back to 0 so VERSION=1.2 still produces a valid
# .rc. An explicitly empty VERSION= degrades to the documented 0.0.0 default
# rather than stamping an empty version string.
VER_CLEAN := $(patsubst v%,%,$(or $(strip $(VERSION)),0.0.0))
VER_NUM   := $(firstword $(subst +, ,$(subst -, ,$(VER_CLEAN))))
V_PARTS   := $(subst ., ,$(VER_NUM))
VER_MAJOR := $(or $(word 1,$(V_PARTS)),0)
VER_MINOR := $(or $(word 2,$(V_PARTS)),0)
VER_PATCH := $(or $(word 3,$(V_PARTS)),0)
VER_BUILD := $(or $(word 4,$(V_PARTS)),0)

WINDRES   ?= windres
RC        := helper.rc
ICON      := assets/helper.ico
BUILD_DIR := build
RES_OBJ   := $(BUILD_DIR)/helper_res.o
VER_STAMP := $(BUILD_DIR)/version.stamp

# Quoting note: windres re-runs its -D flags through a shell when it spawns the
# preprocessor, so plain '"1.2.3"' arrives stripped and breaks the VALUE line.
# The quotes must be BACKSLASH-escaped inside make's single quotes so that
# windres's own shell leaves a real string literal behind.
RCFLAGS   := --include-dir . \
             -DVER_MAJOR=$(VER_MAJOR) -DVER_MINOR=$(VER_MINOR) \
             -DVER_PATCH=$(VER_PATCH) -DVER_BUILD=$(VER_BUILD) \
             -DVER_STRING='\"$(VER_CLEAN)\"'

# Some shells inherit an unwritable TMP (e.g. C:\WINDOWS\); force a known-good
# temp dir for gcc's intermediate files. MSYS2 always provides /tmp, and this
# build only ever runs inside an MSYS2 MINGW64 shell. Override by exporting
# TMPDIR before invoking make.
export TMPDIR := $(if $(TMPDIR),$(TMPDIR),/tmp)
export TMP    := $(TMPDIR)
export TEMP   := $(TMPDIR)

# mingw runtime DLLs to bundle with the shared build
MINGW_BIN ?= /mingw64/bin
RUNTIME_DLLS := libstdc++-6.dll libgcc_s_seh-1.dll libwinpthread-1.dll

DIST_SHARED    := dist
DIST_STATIC    := dist-static
DIST_INSTALLER := dist-installer

# --- Inno Setup compiler location ---
# The GitHub windows-latest runners already have ISCC on PATH, so prefer that;
# a developer machine typically doesn't, so fall back to Inno Setup 6's default
# install location. Either way this is just the *default* — an explicit
# `make installer ISCC=/some/path/ISCC.exe` on the command line (or in the
# environment) always wins, because ?= only assigns when ISCC is still unset.
# $(shell ...) here is lazy (?= makes ISCC recursively-expanded), so this probe
# only runs when a target actually uses $(ISCC), not on every `make` invocation.
ISCC_DEFAULT := C:/Program Files (x86)/Inno Setup 6/ISCC.exe
ISCC         ?= $(if $(shell command -v iscc 2>/dev/null),iscc,$(ISCC_DEFAULT))
ISS          := installer/helper.iss

.PHONY: all shared static installer clean check-toolchain check-iscc FORCE

all: shared static

check-toolchain:
	@command -v $(CXX) >/dev/null 2>&1 || { echo "ERROR: $(CXX) not found on PATH. Open an MSYS2 MINGW64 shell and install: pacman -S mingw-w64-x86_64-gcc"; exit 1; }
	@command -v $(WINDRES) >/dev/null 2>&1 || { echo "ERROR: $(WINDRES) not found on PATH. Open an MSYS2 MINGW64 shell and install: pacman -S mingw-w64-x86_64-binutils"; exit 1; }

check-iscc:
	@command -v "$(ISCC)" >/dev/null 2>&1 || { \
	  echo "ERROR: Inno Setup's ISCC compiler not found (looked for 'iscc' on PATH and at '$(ISCC_DEFAULT)')."; \
	  echo "Install it with:  winget install JRSoftware.InnoSetup"; \
	  echo "or download it from: https://jrsoftware.org/isdl.php"; \
	  echo "Installed somewhere else? Point make at it: make installer ISCC=/path/to/ISCC.exe"; \
	  exit 1; \
	}

# --- Resource object: app icon (IDI_APP == 101) + VERSIONINFO ---
# VERSION is a make variable, not a file, so it cannot be a prerequisite
# directly. The stamp below records the requested version on disk; its recipe
# runs every time but only writes (and so only moves the mtime) when the
# version actually changed. That makes `make static VERSION=x` re-stamp the
# resource without a clean, while an unchanged rebuild stays a no-op.
FORCE:

$(VER_STAMP): FORCE | $(BUILD_DIR)
	@[ -f $@ ] && [ "$$(cat $@)" = '$(VER_CLEAN)' ] || printf '%s' '$(VER_CLEAN)' > $@

$(RES_OBJ): $(RC) $(ICON) $(VER_STAMP) $(MAKEFILE_LIST) | $(BUILD_DIR)
	$(WINDRES) $(RCFLAGS) -i $(RC) -o $@ -O coff

$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

# --- Shared build: helper.exe + 3 runtime DLLs in dist/ ---
shared: check-toolchain $(DIST_SHARED)/helper.exe $(addprefix $(DIST_SHARED)/,$(RUNTIME_DLLS))
	@echo ""
	@echo "Shared build ready:"
	@ls -la $(DIST_SHARED)/
	@echo ""
	@echo "Ship the whole $(DIST_SHARED)/ folder; users run dist/helper.exe."

$(DIST_SHARED)/helper.exe: $(SRC) $(RES_OBJ) | $(DIST_SHARED)
	$(CXX) $(CXXFLAGS) -o $@ $(SRC) $(RES_OBJ) $(LIBS)

$(DIST_SHARED)/%.dll: $(MINGW_BIN)/%.dll | $(DIST_SHARED)
	cp $< $@

$(DIST_SHARED):
	mkdir -p $(DIST_SHARED)

# --- Static build: single fat exe in dist-static/ ---
static: check-toolchain $(DIST_STATIC)/helper.exe
	@echo ""
	@echo "Static build ready:"
	@ls -la $(DIST_STATIC)/
	@echo ""
	@echo "Single-file exe — ship $(DIST_STATIC)/helper.exe alone."

$(DIST_STATIC)/helper.exe: $(SRC) $(RES_OBJ) | $(DIST_STATIC)
	$(CXX) $(CXXFLAGS) -static -static-libgcc -static-libstdc++ \
	  -o $@ $(SRC) $(RES_OBJ) $(LIBS) -lpthread

$(DIST_STATIC):
	mkdir -p $(DIST_STATIC)

# --- Installer: per-user Inno Setup package wrapping the static exe ---
# helper.iss lives in installer/ and its own OutputDir is "..\dist-installer"
# — a path relative to the .iss file, not to make's cwd. Since ISS below is
# given to ISCC as "installer/helper.iss" (relative to this Makefile, i.e.
# windows-helper/), that resolves to windows-helper/dist-installer/ regardless
# of where make itself was invoked from. DIST_INSTALLER must keep matching
# that OutputDir, or the ls/echo below will look in the wrong place.
#
# MSYS2_ARG_CONV_EXCL='*': ISCC.exe is a native Windows program that takes
# Windows-style "/Dname=value" flags, not MSYS-style "-D". MSYS2's runtime
# auto-converts arguments that look like POSIX paths before handing them to a
# non-MSYS executable, and a leading "/" is exactly what that heuristic looks
# for -- so "/DVERSION=0.0.1" gets silently mangled and ISCC sees garbage
# ("You may not specify more than one script filename."). Disabling the
# conversion for this one recipe (rather than exporting it Makefile-wide)
# keeps the workaround scoped to the single command that actually needs it.
installer: check-iscc static
	MSYS2_ARG_CONV_EXCL='*' "$(ISCC)" /DVERSION=$(VER_CLEAN) $(ISS)
	@echo ""
	@echo "Installer ready:"
	@ls -la $(DIST_INSTALLER)/DermoscopeHelper-Setup-$(VER_CLEAN).exe
	@echo ""
	@echo "Ship $(DIST_INSTALLER)/DermoscopeHelper-Setup-$(VER_CLEAN).exe — that's the one users run."

# Windows keeps a running helper.exe's image locked (AV scanners hold brief
# locks too), which makes a first rm fail. Retry once before giving up — the
# second rm is unguarded so a genuinely stuck file still fails loudly rather
# than leaving stale output behind under a "successful" clean.
clean:
	@rm -rf $(DIST_SHARED) $(DIST_STATIC) $(DIST_INSTALLER) $(BUILD_DIR) 2>/dev/null || { \
	  echo "clean: a build output is locked (is helper.exe still running?); retrying..."; \
	  sleep 1; \
	  rm -rf $(DIST_SHARED) $(DIST_STATIC) $(DIST_INSTALLER) $(BUILD_DIR); \
	}

# --- Quick-run helpers (no real dependency tracking; rebuild + run) ---
.PHONY: run run-static
run: shared
	./$(DIST_SHARED)/helper.exe 8080

run-static: static
	./$(DIST_STATIC)/helper.exe 8080
