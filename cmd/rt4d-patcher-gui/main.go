// rt4d-patcher-gui is a small graphical front-end around the patcher
// package. The user picks the original Radtel RT-4D upgrade tool exe,
// the GUI shows what would be patched, and on confirmation writes the
// patched file next to the input with a "_cn" suffix.
//
// Built with Fyne so the same binary works on Windows, macOS and Linux.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"github.com/yourorg/rt4d-patcher/internal/patcher"
)

const (
	windowTitle = "RT-4D Patcher — V3.25 EN → V3.16 CN"
	windowW     = 640.0
	windowH     = 460.0
)

func main() {
	a := app.NewWithID("com.example.rt4d-patcher")
	w := a.NewWindow(windowTitle)

	ui := newPatcherUI(w)
	w.SetContent(ui.root)
	w.Resize(fyne.NewSize(windowW, windowH))
	w.ShowAndRun()
}

// patcherUI wraps every widget so callbacks can update them without
// closing over a tangle of local variables.
type patcherUI struct {
	window fyne.Window
	root   fyne.CanvasObject

	inputPath  *widget.Label
	outputPath *widget.Label
	statusLine *widget.Label
	log        *widget.Entry

	pickBtn  *widget.Button
	patchBtn *widget.Button

	// Cached input data and the calculated output path. The output path
	// is recomputed whenever a new input is picked.
	inputBytes []byte
	inputName  string
	outName    string
}

func newPatcherUI(w fyne.Window) *patcherUI {
	ui := &patcherUI{window: w}

	header := widget.NewLabelWithStyle(
		"Patches the Radtel RT-4D English upgrade tool to work with iRadio DM-UV4R V3.16 Chinese bootloader.",
		fyne.TextAlignLeading,
		fyne.TextStyle{},
	)
	header.Wrapping = fyne.TextWrapWord

	ui.inputPath = widget.NewLabel("(no file selected)")
	ui.inputPath.Wrapping = fyne.TextWrapBreak
	ui.outputPath = widget.NewLabel("")
	ui.outputPath.Wrapping = fyne.TextWrapBreak

	ui.statusLine = widget.NewLabel("Pick an input .exe to begin.")

	// Read-only log; users can scroll and copy from it. MultiLine + Disabled
	// gives us a scrollable text view that's still selectable.
	ui.log = widget.NewMultiLineEntry()
	ui.log.Disable()
	ui.log.SetPlaceHolder("(log output appears here)")
	ui.log.SetMinRowsVisible(12)
	ui.log.Wrapping = fyne.TextWrapWord

	ui.pickBtn = widget.NewButton("Choose input .exe...", ui.onPick)
	ui.patchBtn = widget.NewButton("Patch and save", ui.onPatch)
	ui.patchBtn.Disable()
	aboutBtn := widget.NewButton("About", ui.onAbout)

	buttons := container.NewHBox(ui.pickBtn, ui.patchBtn, aboutBtn)

	form := container.NewVBox(
		header,
		widget.NewSeparator(),
		widget.NewLabel("Input:"),
		ui.inputPath,
		widget.NewLabel("Output:"),
		ui.outputPath,
		widget.NewSeparator(),
		buttons,
		ui.statusLine,
	)

	// Borderlayout: form on top, log fills the rest.
	ui.root = container.NewBorder(form, nil, nil, nil, ui.log)
	return ui
}

// onPick opens the file dialog and loads the chosen file into memory.
// We read the file eagerly so Analyze can show feedback right away.
func (ui *patcherUI) onPick() {
	d := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			ui.showError("Open failed", err)
			return
		}
		if reader == nil {
			// User cancelled.
			return
		}
		defer reader.Close()

		path := reader.URI().Path()
		data, err := os.ReadFile(path)
		if err != nil {
			ui.showError("Read failed", err)
			return
		}

		ui.inputBytes = data
		ui.inputName = path
		ui.outName = autoOutputPath(path)

		ui.inputPath.SetText(path)
		ui.outputPath.SetText(ui.outName)

		ui.analyze()
	}, ui.window)

	// Filter to .exe only by default. Fyne accepts a list of extensions.
	d.SetFilter(storage.NewExtensionFileFilter([]string{".exe", ".EXE"}))
	d.Resize(fyne.NewSize(windowW*0.9, windowH*0.9))
	d.Show()
}

// analyze inspects the currently-loaded input and writes the result into
// the log area. Sets patch-button enabled state accordingly.
func (ui *patcherUI) analyze() {
	ui.log.SetText("")
	ui.appendLog("=== Analyzing %s ===", filepath.Base(ui.inputName))

	res := patcher.Analyze(ui.inputBytes)
	ui.appendLog("File size: %d bytes (%.1f KiB)\n",
		res.FileSize, float64(res.FileSize)/1024)

	if len(res.Found) > 0 {
		ui.appendLog("Patterns found:")
		for _, m := range res.Found {
			ui.appendLog("  [0x%08X] %s", m.Offset, m.Patch.Description)
		}
	}
	if len(res.Missing) > 0 {
		ui.appendLog("")
		ui.appendLog("Patterns NOT found:")
		for _, p := range res.Missing {
			ui.appendLog("  - %s", p.Description)
		}
	}

	ui.appendLog("")
	switch {
	case res.CanPatch():
		ui.statusLine.SetText("Ready to patch. Click \"Patch and save\".")
		ui.patchBtn.Enable()
		ui.appendLog("Ready: file looks like the original V3.25 EN exe.")
	case len(res.Found) > 0:
		ui.statusLine.SetText("Partial match — file may already be patched.")
		ui.patchBtn.Disable()
		ui.appendLog("WARNING: some patterns missing. File may already be patched.")
	default:
		ui.statusLine.SetText("No patterns recognised. Pick a different file.")
		ui.patchBtn.Disable()
		ui.appendLog("This doesn't look like a Radtel RT-4D upgrade tool exe.")
	}
}

// onPatch runs the actual patch, confirming first if the output already
// exists.
func (ui *patcherUI) onPatch() {
	if len(ui.inputBytes) == 0 || ui.outName == "" {
		return
	}

	doWrite := func() {
		res, err := patcher.Apply(ui.inputBytes)
		if err != nil {
			ui.showError("Patch failed", err)
			return
		}
		if err := os.WriteFile(ui.outName, res.Patched, 0o644); err != nil {
			ui.showError("Write failed", err)
			return
		}

		ui.appendLog("")
		ui.appendLog("=== Patched ===")
		for _, m := range res.Applied {
			ui.appendLog("  [0x%08X] %s", m.Offset, m.Patch.Description)
		}
		ui.appendLog("")
		ui.appendLog("Wrote %s (%d bytes)", ui.outName, len(res.Patched))
		ui.statusLine.SetText("Done. Output written next to the input file.")

		dialog.ShowInformation(
			"Patched",
			fmt.Sprintf("Output written to:\n%s", ui.outName),
			ui.window,
		)
	}

	if _, err := os.Stat(ui.outName); err == nil {
		dialog.ShowConfirm(
			"Overwrite existing file?",
			fmt.Sprintf("%s already exists.\nOverwrite it?", ui.outName),
			func(ok bool) {
				if ok {
					doWrite()
				}
			},
			ui.window,
		)
		return
	}
	doWrite()
}

func (ui *patcherUI) onAbout() {
	about := strings.TrimSpace(`
RT-4D Patcher

Converts the Radtel RT-4D V3.25 English upgrade tool .exe into a
variant compatible with the iRadio DM-UV4R bootloader running the
factory V3.16 Chinese firmware.

The two upgrade tools are byte-for-byte identical except for the
checksum BASE used by the bootloader command protocol:
  - V3.25 (English): BASE = 0x48
  - V3.16 (Chinese): BASE = 0x46

This tool patches three hardcoded checksum bytes and one IL-code
constant so the resulting exe transmits packets the V3.16 bootloader
accepts.

Reverse-engineered from open-source Python (RT4D-Editor / jcalado) and
.NET IL disassembly of the upgrade tool. No proprietary code shipped.
`)
	dialog.ShowInformation("About", about, ui.window)
}

func (ui *patcherUI) appendLog(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	if cur := ui.log.Text; cur == "" {
		ui.log.SetText(line)
	} else {
		ui.log.SetText(cur + "\n" + line)
	}
	// Refresh forces redraw; we deliberately don't touch CursorRow because
	// it has no observable effect on a disabled entry (the cursor isn't
	// rendered). Users can scroll manually if the log gets long.
	ui.log.Refresh()
}

func (ui *patcherUI) showError(title string, err error) {
	dialog.ShowError(fmt.Errorf("%s: %w", title, err), ui.window)
	ui.appendLog("ERROR (%s): %v", title, err)
}

// autoOutputPath inserts "_cn" before the extension of input. Shared
// logic with the CLI; duplicated here to keep the GUI a self-contained
// package that doesn't depend on CLI internals.
func autoOutputPath(input string) string {
	dir, file := filepath.Split(input)
	ext := filepath.Ext(file)
	stem := strings.TrimSuffix(file, ext)
	return filepath.Join(dir, stem+"_cn"+ext)
}
