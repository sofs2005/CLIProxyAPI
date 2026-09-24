package api

import (
	"strings"
	"testing"
)

// The sort patch must survive the full injection chain used by
// serveManagementControlPanel: every patch is applied in order to the same
// document, and each keeps its own marker.
func TestManagementPanelPatchesComposeWithCodexResetSort(t *testing.T) {
	panel := []byte("<html><body><div id='root'>panel</div></body></html>")

	patched := injectModelPriceDropdownClipPatch(panel)
	patched = injectCodexFreeRefreshPatch(patched, "codex-token-value")
	patched = injectXAIRefreshPatch(patched, "xai-token-value")
	patched = injectCodexResetSortPatch(patched)

	result := string(patched)
	for _, marker := range []string{
		"__cpa_model_price_dropdown_clip_patch__",
		"__cpa_codex_free_refresh_patch__",
		"__cpa_xai_refresh_patch__",
		"__cpa_codex_reset_sort_patch__",
	} {
		if strings.Count(result, marker) != 1 {
			t.Fatalf("marker %q appears %d times, want exactly once", marker, strings.Count(result, marker))
		}
	}

	// Every patch must land before </body> and leave the closing tag intact.
	if idxBody := strings.LastIndex(result, "</body>"); idxBody < 0 || !strings.HasSuffix(result, "</body></html>") {
		t.Fatalf("panel document tail mangled: %q", result[len(result)-40:])
	}

	// Re-serving the patched document must not stack a second copy.
	again := injectCodexResetSortPatch(patched)
	if strings.Count(string(again), "__cpa_codex_reset_sort_patch__") != 1 {
		t.Fatal("re-injection duplicated the codex reset sort patch")
	}
}
