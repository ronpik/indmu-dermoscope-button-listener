# Windows-only dermoscope helper.
# Built with mingw-w64 GCC (install via MSYS2: pacman -S mingw-w64-x86_64-gcc).
#
# Two build modes:
#   make shared   -> dist/helper.exe + runtime DLLs in dist/ (small exe, 3 DLLs alongside)
#   make static   -> dist-static/helper.exe  (single fat exe, no external DLLs)
#   make all      -> both of the above (default)
#   make clean    -> remove both dist dirs and build/
#
# Versioning:
#   make static VERSION=1.2.3   stamps the exe with that version + the app icon
#   VERSION defaults to 0.0.0. Release tags are vX.Y.Z; pass the tag WITHOUT
#   the leading "v". A pre-release suffix (1.2.3-rc1) is fine — it is kept in
#   the displayed version string and stripped from the numeric fields.
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

DIST_SHARED  := dist
DIST_STATIC  := dist-static

.PHONY: all shared static clean check-toolchain FORCE

all: shared static

check-toolchain:
	@command -v $(CXX) >/dev/null 2>&1 || { echo "ERROR: $(CXX) not found on PATH. Open an MSYS2 MINGW64 shell and install: pacman -S mingw-w64-x86_64-gcc"; exit 1; }
	@command -v $(WINDRES) >/dev/null 2>&1 || { echo "ERROR: $(WINDRES) not found on PATH. Open an MSYS2 MINGW64 shell and install: pacman -S mingw-w64-x86_64-binutils"; exit 1; }

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

# Windows keeps a running helper.exe's image locked (AV scanners hold brief
# locks too), which makes a first rm fail. Retry once before giving up — the
# second rm is unguarded so a genuinely stuck file still fails loudly rather
# than leaving stale output behind under a "successful" clean.
clean:
	@rm -rf $(DIST_SHARED) $(DIST_STATIC) $(BUILD_DIR) 2>/dev/null || { \
	  echo "clean: a build output is locked (is helper.exe still running?); retrying..."; \
	  sleep 1; \
	  rm -rf $(DIST_SHARED) $(DIST_STATIC) $(BUILD_DIR); \
	}

# --- Quick-run helpers (no real dependency tracking; rebuild + run) ---
.PHONY: run run-static
run: shared
	./$(DIST_SHARED)/helper.exe 8080

run-static: static
	./$(DIST_STATIC)/helper.exe 8080
