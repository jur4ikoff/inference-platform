package runtime

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	dockerclient "github.com/moby/moby/client"
)

func TestPullImageClosesResponseStream(t *testing.T) {
	for _, test := range []struct {
		name, response string
		wantError      bool
	}{
		{"success", "{\"status\":\"done\"}\n", false},
		{"stream error", "{\"errorDetail\":{\"message\":\"registry failure\"},\"error\":\"registry failure\"}\n", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader(test.response)}
			transport := dockerTransport(func(request *http.Request) (*http.Response, error) {
				if request.Method != http.MethodPost || request.URL.Path != "/v1.56/images/create" {
					t.Errorf("unexpected request: %s %s", request.Method, request.URL)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": {"application/json"}},
					Body:       body,
					Request:    request,
				}, nil
			})
			apiClient, err := dockerclient.New(
				dockerclient.WithHost("tcp://docker.test:2375"),
				dockerclient.WithAPIVersion("1.56"),
				dockerclient.WithHTTPClient(&http.Client{Transport: transport}),
			)
			if err != nil {
				t.Fatal(err)
			}
			docker := &RuntimeService{client: apiClient}
			defer docker.Close()
			err = docker.PullImage(context.Background(), "alpine:latest")
			if (err != nil) != test.wantError {
				t.Fatalf("unexpected pull error: %v", err)
			}
			if !body.closed {
				t.Fatal("pull response stream was not closed")
			}
		})
	}
}

type dockerTransport func(*http.Request) (*http.Response, error)

func (f dockerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error {
	b.closed = true
	return nil
}
