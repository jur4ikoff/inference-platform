package runtime

import (
	"context"
	"errors"
	"github.com/jur4ikoff/inference-service/internal/config"
	"github.com/jur4ikoff/inference-service/internal/domain"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	dockerclient "github.com/moby/moby/client"
)

// Техника: классы эквивалентности успешного и ошибочного ответа Docker.
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

// Техника: классы эквивалентности ответов Docker API.
func TestContainerOperations(t *testing.T) {
	failure := errors.New("docker unavailable")
	for _, tc := range []struct {
		name     string
		call     func(*RuntimeService) error
		response string
	}{
		{"create", func(r *RuntimeService) error {
			_, e := r.CreateContainer(context.Background(), "image", "model", domain.DeploymentSpec{ProviderType: domain.ModelProviderTypeHuggingFace, ProviderHuggingFace: &domain.HuggingFaceProvider{Repo: "org/model"}})
			return e
		}, `{"Id":"container-1","Warnings":[]}`},
		{"get", func(r *RuntimeService) error { _, e := r.GetContainer(context.Background(), "id"); return e }, `{"Id":"id","State":{"Status":"running"}}`},
		{"start", func(r *RuntimeService) error { return r.StartContainer(context.Background(), "id") }, `{}`}, {"stop", func(r *RuntimeService) error { return r.StopContainer(context.Background(), "id") }, `{}`}, {"remove", func(r *RuntimeService) error { return r.RemoveContainer(context.Background(), "id") }, `{}`}, {"image", func(r *RuntimeService) error { return r.EnsureImage(context.Background(), "image") }, `{"Id":"image"}`},
	} {
		for _, fail := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{true: "/failure", false: "/success"}[fail], func(t *testing.T) {
				transport := dockerTransport(func(req *http.Request) (*http.Response, error) {
					if fail {
						return nil, failure
					}
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(tc.response)), Request: req}, nil
				})
				client, err := dockerclient.New(dockerclient.WithHost("tcp://docker.test:2375"), dockerclient.WithAPIVersion("1.56"), dockerclient.WithHTTPClient(&http.Client{Transport: transport}))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = client.Close() })
				err = tc.call(&RuntimeService{client: client})
				if fail && err == nil || !fail && err != nil {
					t.Fatalf("err=%v", err)
				}
			})
		}
	}
}

// Техника: классы эквивалентности имени контейнера.
func TestEndpointWithDockerNetwork(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		invalid    bool
	}{{"/model", "http://model:8080", false}, {"", "", true}} {
		t.Run(tc.name, func(t *testing.T) {
			info := dockerclient.ContainerInspectResult{}
			info.Container.Name = tc.name
			value, err := (&RuntimeService{runtimeConfig: config.RuntimeServiceConfig{DockerNetwork: "demo"}}).ContainerEndpoint(info)
			if (err != nil) != tc.invalid || value != tc.want {
				t.Fatalf("value=%q err=%v", value, err)
			}
		})
	}
}

// Техника: класс неподдерживаемого типа провайдера.
func TestCreateContainerRejectsProvider(t *testing.T) {
	_, err := (&RuntimeService{}).CreateContainer(context.Background(), "image", "model", domain.DeploymentSpec{ProviderType: "unsupported"})
	if err == nil {
		t.Fatal("unsupported provider accepted")
	}
}

// Техника: классы эквивалентности подключения потока событий.
func TestWatchContainerEvents(t *testing.T) {
	for _, fail := range []bool{true, false} {
		t.Run(map[bool]string{true: "failure", false: "success"}[fail], func(t *testing.T) {
			transport := dockerTransport(func(req *http.Request) (*http.Response, error) {
				if fail {
					return nil, errors.New("unavailable")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
			})
			client, err := dockerclient.New(dockerclient.WithHost("tcp://docker.test:2375"), dockerclient.WithAPIVersion("1.56"), dockerclient.WithHTTPClient(&http.Client{Transport: transport}))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			stream := (&RuntimeService{client: client}).WatchContainerEvents(ctx, "")
			select {
			case err := <-stream.Err:
				if fail && err == nil || !fail && !errors.Is(err, io.EOF) {
					t.Fatalf("err=%v", err)
				}
			case <-ctx.Done():
				t.Fatal("timed out")
			}
		})
	}
}

// Техника: классы эквивалентности открытого и закрытого клиента.
func TestCloseRuntime(t *testing.T) {
	for _, closed := range []bool{true, false} {
		t.Run(map[bool]string{true: "closed", false: "open"}[closed], func(t *testing.T) {
			client, err := dockerclient.New(dockerclient.WithHost("tcp://docker.test:2375"), dockerclient.WithAPIVersion("1.56"))
			if err != nil {
				t.Fatal(err)
			}
			if closed {
				_ = client.Close()
			}
			err = (&RuntimeService{client: client}).Close()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
