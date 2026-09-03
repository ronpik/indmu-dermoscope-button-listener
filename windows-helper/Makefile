# Windows-only dermoscope helper.
# Built with mingw-w64 GCC (install via MSYS2: pacman -S mingw-w64-x86_64-gcc).
#
# Two build modes:
#   make shared   -> dist/helper.exe + runtime DLLs in dist/ (small exe, 3 DLLs alongside)
#   make static   -> dist-static/helper.exe  (single fat exe, no external DLLs)
#   make all      -> both of the above (default)
#   make clean    -> remove both dist dirs
#
# Output layouts:
#   dist/
#     helper.exe                 (~800 KB, links against mingw runtime)
#     libstdc++-6.dll
#     libgcc_s_seh-1.dll
#     libwinpthread-1.dll
#
#   dist-static/
#     helper.exe                 (~4 MB, no external deps — drop anywhere)

CXX       ?= g++
CXXFLAGS  ?= -O2 -Wall -std=c++17
SRC       := helper.cpp
LIBS      := -lstrmiids -lole32 -loleaut32 -luuid -lws2_32

# Some shells inherit an unwritable TMP (e.g. C:\WINDOWS\); force a user-owned
# temp dir for gcc's intermediate files.
export TMPDIR := $(if $(TMPDIR),$(TMPDIR),$(HOME)/tmp)
export TMP    := $(TMPDIR)
export TEMP   := $(TMPDIR)

# mingw runtime DLLs to bundle with the shared build
MINGW_BIN ?= /mingw64/bin
RUNTIME_DLLS := libstdc++-6.dll libgcc_s_seh-1.dll libwinpthread-1.dll

DIST_SHARED  := dist
DIST_STATIC  := dist-static

.PHONY: all shared static clean check-toolchain

all: shared static

check-toolchain:
	@command -v $(CXX) >/dev/null 2>&1 || { echo "ERROR: $(CXX) not found on PATH. Open an MSYS2 MINGW64 shell and install: pacman -S mingw-w64-x86_64-gcc"; exit 1; }

# --- Shared build: helper.exe + 3 runtime DLLs in dist/ ---
shared: check-toolchain $(DIST_SHARED)/helper.exe $(addprefix $(DIST_SHARED)/,$(RUNTIME_DLLS))
	@echo ""
	@echo "Shared build ready:"
	@ls -la $(DIST_SHARED)/
	@echo ""
	@echo "Ship the whole $(DIST_SHARED)/ folder; users run dist/helper.exe."

$(DIST_SHARED)/helper.exe: $(SRC) | $(DIST_SHARED)
	$(CXX) $(CXXFLAGS) -o $@ $(SRC) $(LIBS)

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

$(DIST_STATIC)/helper.exe: $(SRC) | $(DIST_STATIC)
	$(CXX) $(CXXFLAGS) -static -static-libgcc -static-libstdc++ \
	  -o $@ $(SRC) $(LIBS) -lpthread

$(DIST_STATIC):
	mkdir -p $(DIST_STATIC)

clean:
	rm -rf $(DIST_SHARED) $(DIST_STATIC)

# --- Quick-run helpers (no real dependency tracking; rebuild + run) ---
.PHONY: run run-static
run: shared
	./$(DIST_SHARED)/helper.exe 8080

run-static: static
	./$(DIST_STATIC)/helper.exe 8080
