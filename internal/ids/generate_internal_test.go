package ids

import "testing"

// TestGeneratedLengths_ReservedNames_AreUnreachable proves Generate can never
// spell a reserved name: every reserved name is outside the generated length
// range. Random draws cannot show this, because a reachable name would still
// be too unlikely to hit. If the range ever overlaps a reserved name, Generate
// would return an error for that draw instead of retrying (spec 5.1).
func TestGeneratedLengths_ReservedNames_AreUnreachable(t *testing.T) {
	for name := range reservedNames {
		if len(name) >= minGenerated && len(name) <= maxGenerated {
			t.Errorf("reserved name %q has length %d inside the generated range %d-%d", name, len(name), minGenerated, maxGenerated)
		}
	}
}
