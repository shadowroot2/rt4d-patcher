package patcher

import (
	"bytes"
	"testing"
)

// buildFixture returns a byte slice that contains every search pattern
// exactly once, surrounded by filler. Useful for unit-testing the
// patcher without needing the real exe in the repo.
func buildFixture() []byte {
	var buf bytes.Buffer

	buf.WriteString("BEFORE-FILLER-")
	for _, p := range ProtocolPatches {
		buf.Write(p.Search)
		buf.WriteByte(0x00) // separator
	}
	buf.WriteString("MIDDLE-FILLER-")
	for _, p := range ILPatches {
		buf.Write(p.Search)
		buf.WriteByte(0x00)
	}
	buf.WriteString("-AFTER-FILLER")

	return buf.Bytes()
}

func TestAnalyzeOnFixtureFindsAllPatches(t *testing.T) {
	data := buildFixture()
	res := Analyze(data)

	wantFound := len(ProtocolPatches) + len(ILPatches)
	if len(res.Found) != wantFound {
		t.Errorf("Found %d patterns, want %d", len(res.Found), wantFound)
	}
	if len(res.Missing) != 0 {
		t.Errorf("Missing should be empty, got: %+v", res.Missing)
	}
	if !res.CanPatch() {
		t.Error("CanPatch should be true on a complete fixture")
	}
}

func TestAnalyzeOnEmptyFile(t *testing.T) {
	res := Analyze(nil)
	if len(res.Found) != 0 {
		t.Errorf("empty file should have 0 matches, got %d", len(res.Found))
	}
	if len(res.Missing) != len(ProtocolPatches)+len(ILPatches) {
		t.Errorf("empty file should miss every pattern, got %d missing",
			len(res.Missing))
	}
	if res.CanPatch() {
		t.Error("CanPatch should be false on empty input")
	}
}

func TestApplyReplacesAllExpectedBytes(t *testing.T) {
	data := buildFixture()
	original := make([]byte, len(data))
	copy(original, data)

	res, err := Apply(data)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Apply must NOT mutate the input slice.
	if !bytes.Equal(data, original) {
		t.Fatal("Apply mutated the input slice")
	}

	// Result should have the same length.
	if len(res.Patched) != len(original) {
		t.Fatalf("patched length %d differs from input %d",
			len(res.Patched), len(original))
	}

	// Every Search pattern should be gone, every Replace pattern present.
	all := append([]Patch{}, ProtocolPatches...)
	all = append(all, ILPatches...)
	for _, p := range all {
		if bytes.Contains(res.Patched, p.Search) {
			t.Errorf("Patched output still contains Search for %q",
				p.Description)
		}
		if !bytes.Contains(res.Patched, p.Replace) {
			t.Errorf("Patched output missing Replace for %q",
				p.Description)
		}
	}

	// Number of Applied entries should match number of patches.
	if len(res.Applied) != len(all) {
		t.Errorf("Applied count %d, want %d", len(res.Applied), len(all))
	}
}

func TestApplyOnAlreadyPatchedFails(t *testing.T) {
	// Build a fixture, patch it, then try to patch the result again.
	first, err := Apply(buildFixture())
	if err != nil {
		t.Fatalf("first Apply failed: %v", err)
	}

	_, err = Apply(first.Patched)
	if err == nil {
		t.Fatal("Apply on an already-patched file should fail")
	}
}

func TestKnownChecksumValuesMatchProtocol(t *testing.T) {
	// Cross-check that the hardcoded V3.25 → V3.16 mappings are
	// arithmetically what the documented protocol expects.
	//
	//   checksum = (sum_of_command_bytes + BASE) & 0xFF
	//
	// V3.25 BASE = 0x48, V3.16 CN BASE = 0x46. So every Replace byte
	// should be exactly the corresponding Search byte minus 2.
	for _, p := range ProtocolPatches {
		if len(p.Search) != 5 || len(p.Replace) != 5 {
			t.Errorf("%q: expected 5-byte patterns", p.Description)
			continue
		}
		// Bytes 0..3 must be identical (the command payload).
		for i := 0; i < 4; i++ {
			if p.Search[i] != p.Replace[i] {
				t.Errorf("%q: byte %d differs (%02X vs %02X), only checksum should change",
					p.Description, i, p.Search[i], p.Replace[i])
			}
		}
		// Byte 4 is the checksum; should decrease by 2 (0x48 - 0x46).
		if int(p.Search[4])-int(p.Replace[4]) != 2 {
			t.Errorf("%q: checksum delta %d, want 2",
				p.Description, int(p.Search[4])-int(p.Replace[4]))
		}
	}
}

func TestILPatchOnlyChangesBaseByte(t *testing.T) {
	for _, p := range ILPatches {
		if len(p.Search) != len(p.Replace) {
			t.Fatalf("%q: length mismatch", p.Description)
		}
		diffCount := 0
		for i := range p.Search {
			if p.Search[i] != p.Replace[i] {
				diffCount++
			}
		}
		if diffCount != 1 {
			t.Errorf("%q: expected exactly 1 byte change, got %d",
				p.Description, diffCount)
		}
	}
}
