package testutil

import "testing"

// Техника: классы эквивалентности — изменённые и неизменённые данные.
func TestModelBuilderReturnsIndependentSpecs(t *testing.T) {
	builder := NewModelBuilder()
	first := builder.Build()
	first.Spec.ProviderHuggingFace.Repo = "changed"
	second := builder.Build()
	if second.Spec.ProviderHuggingFace.Repo != "org/model" {
		t.Fatalf("builder shared nested data: %+v", second.Spec)
	}
}
