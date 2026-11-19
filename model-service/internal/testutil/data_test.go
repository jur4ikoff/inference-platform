package testutil

import (
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

// Техника: классы эквивалентности — изменённые и неизменённые данные.
func TestModelBuilderReturnsIndependentSpecs(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		builder := NewModelBuilder()
		first := builder.Build()
		first.Spec.ProviderHuggingFace.Repo = "changed"
		second := builder.Build()
		if second.Spec.ProviderHuggingFace.Repo != "org/model" {
			allureCtx.Fatalf("builder shared nested data: %+v", second.Spec)
		}
	})
}
