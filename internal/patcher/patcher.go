// Package patcher converts the Radtel RT-4D English upgrade tool .exe
// files into a form that works against the iRadio DM-UV4R bootloader
// still running the factory V3.16 Chinese firmware.
//
// The two upgrade tools are byte-for-byte identical except for four
// bytes:
//
//   - 3 hardcoded checksums embedded in the .NET resources of the exe
//     (handshake, EE command and erase trigger)
//   - 1 IL-code constant inside the checksum function (BASE: 0x48 → 0x46)
//
// The IL-code change is what mathematically derives the three hardcoded
// values, so in theory patching only the IL constant would be enough.
// In practice the upgrade tool also compares its outgoing packets
// against the hardcoded constants in some paths, so we change all four
// to stay safe.
package patcher

import (
	"bytes"
	"errors"
	"fmt"
)

// Patch describes one byte-pattern replacement to apply to an exe file.
//
// Search and Replace MUST be the same length — the exe contains .NET
// metadata that records offsets, so changing the file size would
// corrupt the assembly.
type Patch struct {
	Description string
	Search      []byte
	Replace     []byte
}

// ProtocolPatches are the three hardcoded protocol byte sequences inside
// the upgrade tool's .NET resources.
//
// These were located by static analysis of the V3.25 upgrade tool
// (RT-4D_Upgrade_V3_25_260205.exe) at known offsets near 0xDD8. Each
// pattern is unique enough that the search has exactly one match in the
// original exe.
var ProtocolPatches = []Patch{
	{
		Description: "Handshake checksum (BASE 0x48 → 0x46)",
		Search:      []byte{0x39, 0x33, 0x05, 0x10, 0xC9},
		Replace:     []byte{0x39, 0x33, 0x05, 0x10, 0xC7},
	},
	{
		Description: "EE command checksum (BASE 0x48 → 0x46)",
		Search:      []byte{0x39, 0x33, 0x05, 0xEE, 0xA7},
		Replace:     []byte{0x39, 0x33, 0x05, 0xEE, 0xA5},
	},
	{
		Description: "Erase trigger checksum (BASE 0x48 → 0x46)",
		Search:      []byte{0x39, 0x33, 0x05, 0x55, 0x0E},
		Replace:     []byte{0x39, 0x33, 0x05, 0x55, 0x0C},
	},
}

// ILPatches modify the checksum function inside the .NET IL code itself.
//
// The original IL sequence is:
//
//	06       ldloc.0          ; running sum
//	1F 48    ldc.i4.s 0x48    ; push BASE (V3.25 = 0x48)
//	58       add
//	D2       conv.u1          ; & 0xFF
//
// We patch the 0x48 to 0x46. The surrounding bytes (06, 58, D2) keep
// the match unique enough that no other place in the binary contains
// the same five-byte sequence.
var ILPatches = []Patch{
	{
		Description: "Checksum BASE constant inside IL (ldc.i4.s 0x48 → 0x46)",
		Search:      []byte{0x06, 0x1F, 0x48, 0x58, 0xD2},
		Replace:     []byte{0x06, 0x1F, 0x46, 0x58, 0xD2},
	},
}

// Match represents one location where a patch was (or could be) applied.
type Match struct {
	Patch  Patch
	Offset int
}

// AnalysisResult bundles everything Analyze finds about an input file.
//
// Found is non-empty when the input is the original V3.25 English exe.
// Missing is non-empty when at least one of the expected patterns isn't
// in the file — usually because the file has already been patched, or
// because the user picked the wrong exe.
type AnalysisResult struct {
	FileSize int
	Found    []Match
	Missing  []Patch
}

// CanPatch reports whether Analyze located every expected pattern.
func (a AnalysisResult) CanPatch() bool {
	return len(a.Missing) == 0 && len(a.Found) > 0
}

// Analyze searches data for every expected patch pattern without
// modifying it.
func Analyze(data []byte) AnalysisResult {
	res := AnalysisResult{FileSize: len(data)}
	all := append([]Patch{}, ProtocolPatches...)
	all = append(all, ILPatches...)

	for _, p := range all {
		offsets := findAll(data, p.Search)
		if len(offsets) == 0 {
			res.Missing = append(res.Missing, p)
			continue
		}
		for _, off := range offsets {
			res.Found = append(res.Found, Match{Patch: p, Offset: off})
		}
	}
	return res
}

// Apply produces a patched copy of data and returns it along with the
// list of locations where bytes were changed.
//
// Returns an error if any patch couldn't be applied at all. Patches
// that find no occurrences but where a sibling patch succeeded are
// reported as warnings via the Skipped field instead of failing the
// whole operation.
type ApplyResult struct {
	Patched []byte
	Applied []Match
	Skipped []Patch
}

// Apply runs every patch in ProtocolPatches and ILPatches against a copy
// of data, returning the patched bytes and a record of what changed.
//
// The function refuses to write if no patches matched at all, which
// almost always means the user picked the wrong file (or a file that
// has already been patched).
func Apply(data []byte) (ApplyResult, error) {
	out := make([]byte, len(data))
	copy(out, data)

	res := ApplyResult{Patched: out}

	all := append([]Patch{}, ProtocolPatches...)
	all = append(all, ILPatches...)

	for _, p := range all {
		if len(p.Search) != len(p.Replace) {
			// Programming error, not user error — fail loudly.
			return ApplyResult{}, fmt.Errorf(
				"internal: patch %q has mismatched lengths (%d/%d)",
				p.Description, len(p.Search), len(p.Replace),
			)
		}

		offsets := findAll(out, p.Search)
		if len(offsets) == 0 {
			res.Skipped = append(res.Skipped, p)
			continue
		}
		for _, off := range offsets {
			copy(out[off:off+len(p.Replace)], p.Replace)
			res.Applied = append(res.Applied, Match{Patch: p, Offset: off})
		}
	}

	if len(res.Applied) == 0 {
		return ApplyResult{}, errors.New(
			"no patches were applied — file may already be patched, or this isn't a Radtel RT-4D upgrade tool exe",
		)
	}
	return res, nil
}

// findAll returns every offset where needle appears in haystack.
func findAll(haystack, needle []byte) []int {
	var offsets []int
	start := 0
	for {
		idx := bytes.Index(haystack[start:], needle)
		if idx < 0 {
			return offsets
		}
		offsets = append(offsets, start+idx)
		// Step by 1 so overlapping matches are caught. The patterns we
		// look for here can't actually overlap, but being defensive
		// costs nothing.
		start += idx + 1
	}
}
