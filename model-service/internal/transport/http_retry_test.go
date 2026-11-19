package transport

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

type testRoundTripper func(*http.Request) (*http.Response, error)

func (f testRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHTTPRetryOnlyForReads(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			t.Run(method, func(t *testing.T) {
				allure.Wrap(t, func(allureCtx *allure.Context) {
					calls := 0
					transport := RetryHTTPReads{Next: testRoundTripper(func(r *http.Request) (*http.Response, error) {
						calls++
						if calls == 1 {
							return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
						}
						return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
					})}
					request, _ := http.NewRequest(method, "http://example.test", nil)
					response, err := transport.RoundTrip(request)
					if err != nil {
						allureCtx.Fatal(err)
					}
					defer response.Body.Close()
					if method == http.MethodGet && (calls != 2 || response.StatusCode != 200) {
						allureCtx.Fatalf("calls=%d status=%d", calls, response.StatusCode)
					}
					if method == http.MethodPost && (calls != 1 || response.StatusCode != 503) {
						allureCtx.Fatalf("calls=%d status=%d", calls, response.StatusCode)
					}
				})
			})
		}
	})
}

func TestHTTPRetryIsBounded(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		calls := 0
		transport := RetryHTTPReads{Next: testRoundTripper(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("unavailable") })}
		request, _ := http.NewRequest(http.MethodGet, "http://example.test", nil)
		_, err := transport.RoundTrip(request)
		if calls != 2 || err == nil {
			allureCtx.Fatalf("calls=%d err=%v", calls, err)
		}
	})
}
