package domain

import (
	"encoding/json"
	"strings"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

// Техника: классы эквивалентности валидных и невалидных спецификаций.
func TestParseModelSpec(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		valid := `{"provider_type":"HUGGING_FACE","provider_hugging_face":{"repo":"Qwen/Qwen2.5-0.5B-Instruct-GGUF:Q4_K_M"}}`
		for _, tc := range []struct {
			name, raw string
			valid     bool
		}{
			{"valid", valid, true}, {"empty", `{}`, false},
			{"unknown provider", `{"provider_type":"OTHER","provider_hugging_face":{"repo":"org/model"}}`, false},
			{"empty repository", `{"provider_type":"HUGGING_FACE","provider_hugging_face":{"repo":""}}`, false},
			{"unknown provider field", `{"provider_type":"HUGGING_FACE","provider_hugging_face":{"repo":"org/model","token":"secret"}}`, false},
			{"unknown field", `{"provider_type":"HUGGING_FACE","provider_hugging_face":{"repo":"org/model"},"extra":true}`, false},
			{"null", `null`, false}, {"trailing value", `{"provider_type":"HUGGING_FACE","provider_hugging_face":{"repo":"org/model"}} null`, false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				allure.Wrap(t, func(allureCtx *allure.Context) {
					spec, canonical, err := ParseModelSpec(json.RawMessage(tc.raw))
					if tc.valid && (err != nil || spec.ProviderHuggingFace == nil || spec.ProviderHuggingFace.Repo != "Qwen/Qwen2.5-0.5B-Instruct-GGUF:Q4_K_M" || string(canonical) != valid) {
						allureCtx.Fatalf("spec=%+v canonical=%s err=%v", spec, canonical, err)
					}
					if !tc.valid && err == nil {
						allureCtx.Fatalf("spec %s was accepted", tc.raw)
					}
				})
			})
		}
	})
}

// Техника: классы эквивалентности разделителей имени.
func TestValidModelNameSeparators(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		for _, tc := range []struct {
			name  string
			valid bool
		}{{"qwen", true}, {"bolshie-qwen", true}, {"qwen-2-small", true}, {"bolshie_qwen", false}, {"_qwen", false}, {"qwen_", false}, {"qwen--small", false}, {"qwen-", false}, {"Qwen", false}} {
			t.Run(tc.name, func(t *testing.T) {
				allure.Wrap(t, func(allureCtx *allure.Context) {
					got := ValidModelName(tc.name)
					if got != tc.valid {
						allureCtx.Fatalf("valid=%v want=%v", got, tc.valid)
					}
				})
			})
		}
	})
}

// Техника: граничные значения длины DNS-имени.
func TestModelNameDNSLength(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		for _, tc := range []struct {
			length int
			valid  bool
		}{{63, true}, {64, false}} {
			t.Run(strings.Repeat("a", tc.length), func(t *testing.T) {
				allure.Wrap(t, func(allureCtx *allure.Context) {
					got := ValidModelName(strings.Repeat("a", tc.length))
					if got != tc.valid {
						allureCtx.Fatalf("length=%d valid=%v", tc.length, got)
					}
				})
			})
		}
	})
}
