# RT-4D Patcher (Go)

Patches the **Radtel RT-4D V3.25 English upgrade tool** `.exe` into a
variant that works with the **iRadio DM-UV4R bootloader running factory
V3.16 Chinese firmware**.

The two upgrade tools are byte-for-byte identical except for the
checksum BASE used by the bootloader command protocol:

| Firmware            | BASE | Handshake byte | Erase byte | EE byte |
|---------------------|------|----------------|------------|---------|
| V3.25 (English)     | 0x48 | `C9`           | `0E`       | `A7`    |
| V3.16 (Chinese)     | 0x46 | `C7`           | `0C`       | `A5`    |

This tool patches three hardcoded checksum bytes and one IL-code
constant inside the exe so the patched program speaks the V3.16
protocol.

## Two front-ends

| Binary               | When to use                                                |
|----------------------|------------------------------------------------------------|
| `rt4d-patcher-cli`   | Scripting; small (single static binary, no dependencies)   |
| `rt4d-patcher-gui`   | Most users — graphical, drag-and-drop friendly             |

Both share the same patch logic in `internal/patcher` and produce
identical output bytes.

## Usage

### CLI

```
rt4d-patcher-cli <input.exe> [output.exe]
rt4d-patcher-cli -analyze <input.exe>
```

If `output.exe` is omitted, the patched file is written next to the
input with a `_cn` suffix before the extension:

```
RT-4D_Upgrade_V3_25.exe   →   RT-4D_Upgrade_V3_25_cn.exe
```

The `-analyze` flag prints what would be patched without writing
anything.

### GUI

Run `rt4d-patcher-gui`, click **Choose input .exe**, verify the analysis
output, then click **Patch and save**.

## Building

Requires Go 1.22 or newer.

**CLI** (no extra dependencies):

```bash
go build ./cmd/rt4d-patcher-cli
```

**GUI** (Fyne):

```bash
go build ./cmd/rt4d-patcher-gui
```

Fyne needs a C toolchain plus OpenGL/X11 development headers when
building on Linux. On Debian/Ubuntu:

```bash
sudo apt-get install gcc libgl1-mesa-dev libxi-dev libxcursor-dev \
                     libxrandr-dev libxinerama-dev xorg-dev
```

On Windows the standard Go toolchain plus MinGW (`gcc.exe` in `PATH`)
is enough; no extra packages required.

Smaller release-mode binaries:

```bash
go build -ldflags="-s -w" -trimpath ./cmd/rt4d-patcher-cli
go build -ldflags="-s -w -H=windowsgui" -trimpath ./cmd/rt4d-patcher-gui   # Windows: no console window
```

Cross-compile the CLI from Linux to Windows (no CGO needed):

```bash
GOOS=windows GOARCH=amd64 go build -o rt4d-patcher-cli.exe ./cmd/rt4d-patcher-cli
```

The GUI uses CGO so cross-compilation is more involved; see Fyne's
docs at <https://docs.fyne.io/started/cross-compiling>.

## Tests

```bash
go test ./...
```

The `patcher` package has a hand-built byte fixture so tests don't
require shipping a real upgrade tool exe in the repo.

## Project layout

```
cmd/rt4d-patcher-cli/main.go    Plain CLI (no GUI deps)
cmd/rt4d-patcher-gui/main.go    Fyne-based GUI front-end
internal/patcher/patcher.go     Patch definitions + Apply / Analyze
internal/patcher/patcher_test.go Unit tests for the patch logic
```

## Disclaimer

Use at your own risk. Always keep a working copy of the original
Chinese V3.16 upgrade tool for rollback. Power-down or interruption
during a flash can brick the radio.
