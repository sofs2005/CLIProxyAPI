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

func TestInjectModelPriceDropdownClipPatch_IncludesSelectModelFallbackLabels(t *testing.T) {
	input := []byte("<html><body><div>content</div></body></html>")
	out := injectModelPriceDropdownClipPatch(input)
	result := string(out)

	for _, needle := range []string{
		"\\u9009\\u62e9\\u6a21\\u578b",
		"select model",
		"getBoundingClientRect",
	} {
		if !strings.Contains(result, needle) {
			t.Fatalf("expected model price dropdown clip patch to include fallback label %q", needle)
		}
	}
}

func TestInjectModelPriceDropdownClipPatch_DoesNotRegisterGlobalTriggerHooks(t *testing.T) {
	input := []byte("<html><body><div>content</div></body></html>")
	out := injectModelPriceDropdownClipPatch(input)
	result := string(out)

	for _, needle := range []string{
		"pointerdown",
		"focusin",
		"activeTrigger",
		"data-radix-popper-content-wrapper",
		"OVERLAY_SELECTOR",
	} {
		if strings.Contains(result, needle) {
			t.Fatalf("expected model price dropdown clip patch to avoid global trigger hook %q", needle)
		}
	}
}

func TestInjectUsageWarmupPatch_InsertsBeforeBodyClose(t *testing.T) {
	input := []byte("<html><body><div>content</div></body></html>")
	out := injectUsageWarmupPatch(input)
	result := string(out)

	if !strings.Contains(result, "__cpa_usage_warmup_patch__") {
		t.Fatal("expected usage warmup patch marker in output")
	}

	idxBody := strings.LastIndex(result, "</body>")
	idxMarker := strings.Index(result, "__cpa_usage_warmup_patch__")
	if idxBody < 0 || idxMarker < 0 || idxMarker > idxBody {
		t.Fatal("expected usage warmup patch injected before </body>")
	}
}

func TestInjectUsageWarmupPatch_OnlyInjectsOnce(t *testing.T) {
	input := []byte("<html><body><div>content</div></body></html>")
	first := injectUsageWarmupPatch(input)
	second := injectUsageWarmupPatch(first)
	result := string(second)

	if strings.Count(result, "__cpa_usage_warmup_patch__") != 1 {
		t.Fatal("expected usage warmup patch marker to appear exactly once")
	}
}

func TestInjectUsageWarmupPatch_AppendsWhenBodyMissing(t *testing.T) {
	input := []byte("<html><div>content</div></html>")
	out := injectUsageWarmupPatch(input)
	result := string(out)

	if !strings.Contains(result, "__cpa_usage_warmup_patch__") {
		t.Fatal("expected usage warmup patch marker in output")
	}
	if !strings.HasSuffix(result, "</script>") {
		t.Fatal("expected usage warmup patch appended to document end when </body> is missing")
	}
}

func TestInjectUsagePaginationPatch_InsertsExpectedHooks(t *testing.T) {
	input := []byte("<html><body><div>content</div></body></html>")
	out := injectUsagePaginationPatch(input)
	result := string(out)

	for _, needle := range []string{
		"__cpa_usage_pagination_patch__",
		"api_page_size",
		"detail_page_size",
		"request_details_page",
		"sessionStorage",
		"XMLHttpRequest",
		"MutationObserver",
		"cpa-usage-pagination-fallback-root",
		"cpa_pagination",
		"loadUsagePage",
		"/v0/management/usage",
		"responseType",
		"getBoundingClientRect",
	} {
		if !strings.Contains(result, needle) {
			t.Fatalf("expected usage pagination patch to include %q", needle)
		}
	}

	for _, needle := range []string{
		"window.location.reload()",
	} {
		if strings.Contains(result, needle) {
			t.Fatalf("expected usage pagination patch to avoid %q", needle)
		}
	}
}

func TestManagementPatchChain_ContainsBothMarkers(t *testing.T) {
	input := []byte("<html><body><div>content</div></body></html>")
	out := injectAuthFilesWarningFilterPatch(input)
	out = injectModelPriceDropdownClipPatch(out)
	out = injectUsageWarmupPatch(out)
	out = injectUsagePaginationPatch(out)
	result := string(out)

	if !strings.Contains(result, "__cpa_auth_warning_filter_patch__") {
		t.Fatal("expected auth warning patch marker in chained output")
	}
	if !strings.Contains(result, "__cpa_model_price_dropdown_clip_patch__") {
		t.Fatal("expected model price dropdown patch marker in chained output")
	}
	if !strings.Contains(result, "__cpa_usage_warmup_patch__") {
		t.Fatal("expected usage warmup patch marker in chained output")
	}
	if !strings.Contains(result, "__cpa_usage_pagination_patch__") {
		t.Fatal("expected usage pagination patch marker in chained output")
	}
}
