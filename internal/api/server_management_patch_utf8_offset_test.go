package api

import (
	"bytes"
	"strings"
	"testing"
)

// turkishDottedCapitalI is U+0130, encoded as two bytes (0xC4 0xB0) but
// lowercased by unicode.ToLower into 'i' (U+0069), which is one byte. The
// management panel's i18n contains hundreds of these, which is what made
// bytes.ToLower(html) shorter than html and pushed the computed </body> offset
// back into the trailing <style> block.
const turkishDottedCapitalI = "\u0130"

// turkishPanelFixture builds a management-panel-shaped document: a head full of
// 'İ' characters followed by a <style> block, then a short body. The 'İ' run
// comes before the style block and the tail after </style> is much shorter than
// the resulting drift, so an offset computed on bytes.ToLower(html) lands
// inside the CSS text.
func turkishPanelFixture(t *testing.T) []byte {
	t.Helper()

	var b strings.Builder
	b.WriteString("<html><head>")
	b.WriteString(strings.Repeat(turkishDottedCapitalI, 300))
	b.WriteString("<style>\n")
	for i := 0; i < 40; i++ {
		b.WriteString("body{font-size:clamp(12px,1vw,16px);line-height:1.5}\n")
	}
	b.WriteString("</style></head><body><div id=\"root\">panel</div></body></html>")
	return []byte(b.String())
}

// assertPatchOutsideStyleBlock checks that the patch carrying marker sits in the
// document tail: after the last </style> (so it is not parsed as CSS text) and
// before </body>, with the closing tags intact.
func assertPatchOutsideStyleBlock(t *testing.T, doc []byte, marker string) {
	t.Helper()

	text := string(doc)
	idxMarker := strings.Index(text, marker)
	if idxMarker < 0 {
		t.Fatalf("expected patch marker %q in output", marker)
	}
	idxBody := strings.LastIndex(text, "</body>")
	if idxBody < 0 {
		t.Fatalf("document lost its </body> closing tag (tail %q)", tailOf(text, 60))
	}
	if idxMarker > idxBody {
		t.Fatalf("patch marker %q landed after </body> (marker %d, body %d)", marker, idxMarker, idxBody)
	}
	if idxStyleClose := strings.LastIndex(text, "</style>"); idxStyleClose >= 0 && idxMarker < idxStyleClose {
		t.Fatalf("patch marker %q landed before the last </style> (marker %d, style close %d): the patch would be parsed as CSS", marker, idxMarker, idxStyleClose)
	}
	if !strings.HasSuffix(text, "</body></html>") {
		t.Fatalf("document tail mangled: %q", tailOf(text, 40))
	}
}

func tailOf(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func TestLastIndexFold_MatchesLiteralOffset(t *testing.T) {
	tests := []struct {
		name     string
		haystack string
		needle   string
		want     int
	}{
		{
			name:     "lowercase tail",
			haystack: "<html><body>panel</body></html>",
			needle:   "</body>",
			want:     strings.Index("<html><body>panel</body></html>", "</body>"),
		},
		{
			name:     "uppercase tail",
			haystack: "<html><body>panel</BODY></html>",
			needle:   "</body>",
			want:     strings.Index("<html><body>panel</BODY></html>", "</BODY>"),
		},
		{
			name:     "mixed case tail",
			haystack: "<html><body>panel</BoDy></html>",
			needle:   "</body>",
			want:     strings.Index("<html><body>panel</BoDy></html>", "</BoDy>"),
		},
		{
			name:     "turkish capital I before needle",
			haystack: "<html><head>" + strings.Repeat(turkishDottedCapitalI, 7) + "</head><body>panel</body></html>",
			needle:   "</body>",
			want:     strings.Index("<html><head>"+strings.Repeat(turkishDottedCapitalI, 7)+"</head><body>panel</body></html>", "</body>"),
		},
		{
			name:     "needle absent",
			haystack: "<html><body>panel</html>",
			needle:   "</body>",
			want:     -1,
		},
		{
			name:     "needle longer than haystack",
			haystack: "</body",
			needle:   "</body>",
			want:     -1,
		},
		{
			name:     "empty haystack",
			haystack: "",
			needle:   "</body>",
			want:     -1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			haystack := []byte(tc.haystack)
			needle := []byte(tc.needle)
			got := lastIndexFold(haystack, needle)
			if got != tc.want {
				t.Fatalf("lastIndexFold(%q, %q) = %d, want %d", tc.haystack, tc.needle, got, tc.want)
			}
			if got >= 0 && !bytes.EqualFold(haystack[got:got+len(needle)], needle) {
				t.Fatalf("lastIndexFold(%q, %q) returned %d which does not point at a case-insensitive match", tc.haystack, tc.needle, got)
			}
		})
	}
}

func TestLastIndexFold_IgnoresUTF8LengthDrift(t *testing.T) {
	html := turkishPanelFixture(t)

	folded := bytes.ToLower(html)
	if len(folded) == len(html) {
		t.Fatalf("fixture must shrink under bytes.ToLower to reproduce the bug (html %d bytes, folded %d bytes)", len(html), len(folded))
	}

	want := bytes.LastIndex(html, []byte("</body>"))
	if want < 0 {
		t.Fatal("fixture must contain </body>")
	}

	if got := lastIndexFold(html, []byte("</body>")); got != want {
		t.Fatalf("lastIndexFold on original bytes = %d, want %d", got, want)
	}
	if got := lastIndexFold(html, []byte("</BODY>")); got != want {
		t.Fatalf("lastIndexFold with uppercase needle = %d, want %d", got, want)
	}

	// The pre-fix algorithm: the offset taken from the folded copy is shifted
	// backwards and lands inside the <style> block, which is exactly the
	// reported bug.
	buggy := bytes.LastIndex(folded, []byte("</body>"))
	if buggy == want {
		t.Fatalf("expected the folded offset to differ from the real offset, both are %d", want)
	}
	styleClose := bytes.Index(html, []byte("</style>"))
	styleOpen := bytes.Index(html, []byte("<style>"))
	if styleOpen < 0 || styleClose < 0 || styleClose <= styleOpen {
		t.Fatalf("fixture must contain a <style> block (open %d, close %d)", styleOpen, styleClose)
	}
	if buggy <= styleOpen || buggy >= styleClose {
		t.Fatalf("expected the folded offset %d to land inside the <style> block (%d..%d)", buggy, styleOpen, styleClose)
	}
}

func TestManagementPanelPatchesLandOutsideStyleBlockWithTurkishCapitalI(t *testing.T) {
	tests := []struct {
		name   string
		inject func([]byte) []byte
		marker string
	}{
		{
			name:   "model price dropdown clip",
			inject: injectModelPriceDropdownClipPatch,
			marker: "__cpa_model_price_dropdown_clip_patch__",
		},
		{
			name:   "codex free refresh",
			inject: func(html []byte) []byte { return injectCodexFreeRefreshPatch(html, "codex-token-value") },
			marker: "__cpa_codex_free_refresh_patch__",
		},
		{
			name:   "xai refresh",
			inject: func(html []byte) []byte { return injectXAIRefreshPatch(html, "xai-token-value") },
			marker: "__cpa_xai_refresh_patch__",
		},
		{
			name:   "codex reset sort",
			inject: injectCodexResetSortPatch,
			marker: "__cpa_codex_reset_sort_patch__",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			html := turkishPanelFixture(t)
			out := tc.inject(html)

			if !bytes.Contains(out, []byte(turkishDottedCapitalI)) {
				t.Fatal("expected the original İ characters to survive patching unchanged")
			}
			if !strings.Contains(string(out), "</style></head><body>") {
				t.Fatal("expected the style block and body opening tag to stay intact")
			}
			assertPatchOutsideStyleBlock(t, out, tc.marker)

			// Re-injection must stay idempotent on this document too.
			if again := tc.inject(out); strings.Count(string(again), tc.marker) != 1 {
				t.Fatalf("expected marker %q exactly once after re-injection, got %d", tc.marker, strings.Count(string(again), tc.marker))
			}
		})
	}
}

// TestManagementPanelPatchChainWithTurkishCapitalI mirrors the injection chain
// used by serveManagementControlPanel on a document whose bytes shrink under
// bytes.ToLower. Every patch must end up as a real <script> element after the
// last </style>, not as CSS text.
func TestManagementPanelPatchChainWithTurkishCapitalI(t *testing.T) {
	html := turkishPanelFixture(t)

	patched := injectModelPriceDropdownClipPatch(html)
	patched = injectCodexFreeRefreshPatch(patched, "codex-token-value")
	patched = injectXAIRefreshPatch(patched, "xai-token-value")
	patched = injectCodexResetSortPatch(patched)

	result := string(patched)
	markers := []string{
		"__cpa_model_price_dropdown_clip_patch__",
		"__cpa_codex_free_refresh_patch__",
		"__cpa_xai_refresh_patch__",
		"__cpa_codex_reset_sort_patch__",
	}
	for _, marker := range markers {
		if strings.Count(result, marker) != 1 {
			t.Fatalf("marker %q appears %d times, want exactly once", marker, strings.Count(result, marker))
		}
		assertPatchOutsideStyleBlock(t, patched, marker)
	}

	// Four patch scripts must reach the browser as script elements.
	if got := strings.Count(result, "<script"); got != len(markers) {
		t.Fatalf("expected %d <script> elements in the patched document, got %d", len(markers), got)
	}
}
