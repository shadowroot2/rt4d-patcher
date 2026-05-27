// rt4d-patcher-cli converts a Radtel RT-4D English upgrade tool .exe
// into a variant that works with the iRadio DM-UV4R bootloader on
// factory V3.16 Chinese firmware.
//
// Usage:
//
//	rt4d-patcher-cli <input.exe> [output.exe]
//
// If output.exe isn't given, the tool appends a "_cn" suffix to the
// input filename (before the .exe extension).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shadowroot2/rt4d-patcher/internal/patcher"
)

func main() {
	// Custom usage so the help text is self-explanatory.
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `rt4d-patcher-cli - patch Radtel RT-4D upgrade tool for iRadio DM-UV4R V3.16 Chinese bootloader

Usage:
  %s [-analyze] <input.exe> [output.exe]

If output.exe is omitted, the result is written next to input.exe with
a "_cn" suffix before the .exe extension (e.g. "RT-4D_Upgrade_V3_25.exe"
becomes "RT-4D_Upgrade_V3_25_cn.exe").

Flags:
`, os.Args[0])
		flag.PrintDefaults()
	}

	analyzeOnly := flag.Bool("analyze", false,
		"only report what would be patched, don't write any output")
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(2)
	}

	inputPath := flag.Arg(0)
	data, err := os.ReadFile(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read %s: %v\n", inputPath, err)
		os.Exit(1)
	}

	if *analyzeOnly {
		printAnalysis(patcher.Analyze(data))
		return
	}

	// Pick output path: CLI arg overrides, otherwise auto-suffix.
	var outPath string
	if flag.NArg() >= 2 {
		outPath = flag.Arg(1)
	} else {
		outPath = autoOutputPath(inputPath)
	}

	if sameFile(inputPath, outPath) {
		fmt.Fprintf(os.Stderr,
			"refusing to overwrite the input file (%s)\n", inputPath)
		os.Exit(1)
	}

	res, err := patcher.Apply(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "patch failed: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outPath, res.Patched, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "cannot write %s: %v\n", outPath, err)
		os.Exit(1)
	}

	fmt.Printf("Applied %d patch(es):\n", len(res.Applied))
	for _, m := range res.Applied {
		fmt.Printf("  [0x%08X] %s\n", m.Offset, m.Patch.Description)
	}
	if len(res.Skipped) > 0 {
		fmt.Println("\nSkipped (pattern not found):")
		for _, p := range res.Skipped {
			fmt.Printf("  - %s\n", p.Description)
		}
	}
	fmt.Printf("\nWrote %s (%d bytes)\n", outPath, len(res.Patched))
}

// autoOutputPath inserts "_cn" before the file extension of input. For
// inputs without an extension we just append "_cn".
//
//	"foo.exe"     → "foo_cn.exe"
//	"foo"         → "foo_cn"
//	"path/foo.exe"→ "path/foo_cn.exe"
func autoOutputPath(input string) string {
	dir, file := filepath.Split(input)
	ext := filepath.Ext(file)
	stem := strings.TrimSuffix(file, ext)
	return filepath.Join(dir, stem+"_cn"+ext)
}

// sameFile reports whether two path strings refer to the same on-disk
// file. We can't always use os.SameFile (b might not exist yet), so
// fall back to comparing cleaned absolute paths.
func sameFile(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		// Best-effort comparison.
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return filepath.Clean(absA) == filepath.Clean(absB)
}

func printAnalysis(res patcher.AnalysisResult) {
	fmt.Printf("File size: %d bytes\n\n", res.FileSize)

	if len(res.Found) > 0 {
		fmt.Println("Patterns found:")
		for _, m := range res.Found {
			fmt.Printf("  [0x%08X] %s\n", m.Offset, m.Patch.Description)
		}
	}
	if len(res.Missing) > 0 {
		fmt.Println("\nMissing patterns:")
		for _, p := range res.Missing {
			fmt.Printf("  - %s\n", p.Description)
			fmt.Printf("    (searched for: % X)\n", p.Search)
		}
	}

	fmt.Println()
	if res.CanPatch() {
		fmt.Println("Result: file CAN be patched.")
	} else if len(res.Found) > 0 {
		fmt.Println("Result: file is partially patched or modified. Patch may still work but verify carefully.")
	} else {
		fmt.Println("Result: nothing to patch — wrong file or already patched.")
	}
}
