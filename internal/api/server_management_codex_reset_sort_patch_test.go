package api

import (
	"strings"
	"testing"
)

func TestInjectCodexResetSortPatch_InsertsBeforeBodyClose(t *testing.T) {
	input := []byte("<html><body><div>content</div></body></html>")
	out := injectCodexResetSortPatch(input)
	result := string(out)

	if !strings.Contains(result, "__cpa_codex_reset_sort_patch__") {
		t.Fatal("expected codex reset sort patch marker in output")
	}
	idxBody := strings.LastIndex(result, "</body>")
	idxMarker := strings.Index(result, "__cpa_codex_reset_sort_patch__")
	if idxBody < 0 || idxMarker < 0 || idxMarker > idxBody {
		t.Fatal("expected codex reset sort patch to be injected before </body>")
	}
}

func TestInjectCodexResetSortPatch_OnlyInjectsOnce(t *testing.T) {
	input := []byte("<html><body><div>content</div></body></html>")
	first := injectCodexResetSortPatch(input)
	second := injectCodexResetSortPatch(first)

	if strings.Count(string(second), "__cpa_codex_reset_sort_patch__") != 1 {
		t.Fatal("expected codex reset sort patch marker to appear exactly once")
	}
}

func TestInjectCodexResetSortPatch_AppendsWhenBodyMissing(t *testing.T) {
	input := []byte("<html><div>content</div></html>")
	out := injectCodexResetSortPatch(input)
	result := string(out)

	if !strings.Contains(result, "__cpa_codex_reset_sort_patch__") {
		t.Fatal("expected codex reset sort patch marker in output")
	}
	if !strings.HasSuffix(result, "</script>") {
		t.Fatal("expected codex reset sort patch appended to document end when </body> is missing")
	}
}

func TestInjectCodexResetSortPatch_UsesAuthFileFieldSignature(t *testing.T) {
	input := []byte("<html><body></body></html>")
	result := string(injectCodexResetSortPatch(input))

	for _, needle := range []string{
		"codex_reset_at",
		"Array.prototype.sort",
		"looksLikeAuthFileList",
		"isDefaultComparator",
		"refineCodexOrder",
	} {
		if !strings.Contains(result, needle) {
			t.Fatalf("expected codex reset sort patch to include %q", needle)
		}
	}
}

func TestInjectCodexResetSortPatch_DoesNotEmbedRefreshToken(t *testing.T) {
	input := []byte("<html><body></body></html>")
	result := string(injectCodexResetSortPatch(input))

	if strings.Contains(result, "__CPA_CODEX_REFRESH_TOKEN__") {
		t.Fatal("expected codex reset sort patch to carry no action token placeholder")
	}
	if strings.Contains(result, "X-Codex-Refresh-Token") {
		t.Fatal("expected codex reset sort patch to make no authenticated API call")
	}
}
