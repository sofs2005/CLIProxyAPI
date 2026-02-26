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

func TestInjectModelPriceDropdownClipPatch_InsertsBeforeBodyClose(t *testing.T) {
	input := []byte("<html><body><div>content</div></body></html>")
	out := injectModelPriceDropdownClipPatch(input)
	result := string(out)

	if !strings.Contains(result, "__cpa_model_price_dropdown_clip_patch__") {
		t.Fatal("expected model price dropdown clip patch marker in output")
	}

	idxBody := strings.LastIndex(result, "</body>")
	idxMarker := strings.Index(result, "__cpa_model_price_dropdown_clip_patch__")
	if idxBody < 0 || idxMarker < 0 || idxMarker > idxBody {
		t.Fatal("expected model price dropdown clip patch to be injected before </body>")
	}
}

func TestInjectModelPriceDropdownClipPatch_OnlyInjectsOnce(t *testing.T) {
	input := []byte("<html><body><div>content</div></body></html>")
	first := injectModelPriceDropdownClipPatch(input)
	second := injectModelPriceDropdownClipPatch(first)
	result := string(second)

	if strings.Count(result, "__cpa_model_price_dropdown_clip_patch__") != 1 {
		t.Fatal("expected model price dropdown clip patch marker to appear exactly once")
	}
}

func TestInjectModelPriceDropdownClipPatch_AppendsWhenBodyMissing(t *testing.T) {
	input := []byte("<html><div>content</div></html>")
	out := injectModelPriceDropdownClipPatch(input)
	result := string(out)

	if !strings.Contains(result, "__cpa_model_price_dropdown_clip_patch__") {
		t.Fatal("expected model price dropdown clip patch marker in output")
	}
	if !strings.HasSuffix(result, "</script>") {
		t.Fatal("expected model price dropdown clip patch appended to document end when </body> is missing")
	}
}

func TestManagementPatchChain_ContainsBothMarkers(t *testing.T) {
	input := []byte("<html><body><div>content</div></body></html>")
	out := injectAuthFilesWarningFilterPatch(input)
	out = injectModelPriceDropdownClipPatch(out)
	result := string(out)

	if !strings.Contains(result, "__cpa_auth_warning_filter_patch__") {
		t.Fatal("expected auth warning patch marker in chained output")
	}
	if !strings.Contains(result, "__cpa_model_price_dropdown_clip_patch__") {
		t.Fatal("expected model price dropdown patch marker in chained output")
	}
}
