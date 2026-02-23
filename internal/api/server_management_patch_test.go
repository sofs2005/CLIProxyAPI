package api

import (
	"strings"
	"testing"
)

func TestInjectAuthFilesWarningFilterPatch_InsertsBeforeBodyClose(t *testing.T) {
	input := []byte("<html><body><div>content</div></body></html>")
	out := injectAuthFilesWarningFilterPatch(input)
	result := string(out)

	if !strings.Contains(result, "__cpa_auth_warning_filter_patch__") {
		t.Fatal("expected warning filter patch marker in output")
	}

	idxBody := strings.LastIndex(result, "</body>")
	idxMarker := strings.Index(result, "__cpa_auth_warning_filter_patch__")
	if idxBody < 0 || idxMarker < 0 || idxMarker > idxBody {
		t.Fatal("expected patch script to be injected before </body>")
	}
}

func TestInjectAuthFilesWarningFilterPatch_OnlyInjectsOnce(t *testing.T) {
	input := []byte("<html><body><div>content</div></body></html>")
	first := injectAuthFilesWarningFilterPatch(input)
	second := injectAuthFilesWarningFilterPatch(first)
	result := string(second)

	if strings.Count(result, "__cpa_auth_warning_filter_patch__") != 1 {
		t.Fatal("expected patch marker to appear exactly once")
	}
}

func TestInjectAuthFilesWarningFilterPatch_AppendsWhenBodyMissing(t *testing.T) {
	input := []byte("<html><div>content</div></html>")
	out := injectAuthFilesWarningFilterPatch(input)
	result := string(out)

	if !strings.Contains(result, "__cpa_auth_warning_filter_patch__") {
		t.Fatal("expected warning filter patch marker in output")
	}
	if !strings.HasSuffix(result, "</script>") {
		t.Fatal("expected patch script appended to document end when </body> is missing")
	}
}

