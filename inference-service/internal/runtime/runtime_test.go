package runtime

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/jur4ikoff/inference-service/internal/config"
	"github.com/jur4ikoff/inference-service/internal/domain"

	dockerclient "github.com/moby/moby/client"
)

// Техника: разбиение на классы эквивалентности успешного и ошибочного ответа Docker.
func TestPullImageClosesResponseStream(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		for _, test := range []struct {
			name, response string
			wantError      bool
		}{
			{"success", "{\"status\":\"done\"}\n", false},
			{"stream error", "{\"errorDetail\":{\"message\":\"registry failure\"},\"error\":\"registry failure\"}\n", true},
		} {
			t.Run(test.name, func(t *testing.T) {
				allure.Wrap(t, func(allureCtx *allure.Context) {
					body := &trackedBody{Reader: strings.NewReader(test.response)}
					transport := dockerTransport(func(request *http.Request) (*http.Response, error) {
						if request.Method != http.MethodPost || request.URL.Path != "/v1.56/images/create" {
							allureCtx.Errorf("unexpected request: %s %s", request.Method, request.URL)
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
						allureCtx.Fatal(err)
					}
					docker := &RuntimeService{client: apiClient}
					defer docker.Close()
					err = docker.PullImage(context.Background(), "alpine:latest")
					if (err != nil) != test.wantError {
						allureCtx.Fatalf("unexpected pull error: %v", err)
					}
					if !body.closed {
						allureCtx.Fatal("pull response stream was not closed")
					}
				})
			})
		}
	})
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

// Техника: разбиение на классы эквивалентности имени контейнера.
func TestEndpointWithDockerNetwork(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		for _, tc := range []struct {
			name, want string
			invalid    bool
		}{{"/model", "http://model:8080", false}, {"", "", true}} {
			t.Run(tc.name, func(t *testing.T) {
				allure.Wrap(t, func(allureCtx *allure.Context) {
					info := dockerclient.ContainerInspectResult{}
					info.Container.Name = tc.name
					value, err := (&RuntimeService{runtimeConfig: config.RuntimeServiceConfig{DockerNetwork: "demo"}}).ContainerEndpoint(info)
					if (err != nil) != tc.invalid || value != tc.want {
						allureCtx.Fatalf("value=%q err=%v", value, err)
					}
				})
			})
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай неподдерживаемого типа провайдера.
func TestCreateContainerRejectsProvider(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		_, err := (&RuntimeService{}).CreateContainer(context.Background(), "image", "model", domain.DeploymentSpec{ProviderType: "unsupported"})
		if err == nil {
			allureCtx.Fatal("unsupported provider accepted")
		}
	})
}

func watchTestRuntime(t *allure.Context, transport dockerTransport) *RuntimeService {
	t.Helper()
	client, err := dockerclient.New(dockerclient.WithHost("tcp://docker.test:2375"), dockerclient.WithAPIVersion("1.56"), dockerclient.WithHTTPClient(&http.Client{Transport: transport}))
	if err != nil {
		t.Fatal(err)
	}
	t.T().Cleanup(func() { _ = client.Close() })
	return &RuntimeService{client: client}
}

// Техника: разбиение на классы эквивалентности; случай успешно открытого потока событий.
func TestWatchContainerEventsReadsToEOF(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		runtime := watchTestRuntime(allureCtx, dockerTransport(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
		}))
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		stream := runtime.WatchContainerEvents(ctx, "")
		select {
		case err := <-stream.Err:
			if !errors.Is(err, io.EOF) {
				allureCtx.Fatalf("err=%v", err)
			}
		case <-ctx.Done():
			allureCtx.Fatal("timed out")
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай ошибки подключения к Docker.
func TestWatchContainerEventsReportsConnectionError(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		failure := errors.New("unavailable")
		runtime := watchTestRuntime(allureCtx, dockerTransport(func(*http.Request) (*http.Response, error) { return nil, failure }))
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		stream := runtime.WatchContainerEvents(ctx, "")
		select {
		case err := <-stream.Err:
			if !errors.Is(err, failure) {
				allureCtx.Fatalf("err=%v", err)
			}
		case <-ctx.Done():
			allureCtx.Fatal("timed out")
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай открытого клиента.
func TestCloseRuntimeClosesOpenClient(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		client, err := dockerclient.New(dockerclient.WithHost("tcp://docker.test:2375"), dockerclient.WithAPIVersion("1.56"))
		if err != nil {
			allureCtx.Fatal(err)
		}
		if err := (&RuntimeService{client: client}).Close(); err != nil {
			allureCtx.Fatal(err)
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай уже закрытого клиента.
func TestCloseRuntimeAllowsRepeatedClose(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		client, err := dockerclient.New(dockerclient.WithHost("tcp://docker.test:2375"), dockerclient.WithAPIVersion("1.56"))
		if err != nil {
			allureCtx.Fatal(err)
		}
		if err := client.Close(); err != nil {
			allureCtx.Fatal(err)
		}
		if err := (&RuntimeService{client: client}).Close(); err != nil {
			allureCtx.Fatal(err)
		}
	})
}

func runtimeWithDockerResponse(t *allure.Context, response string, transportErr error) *RuntimeService {
	t.Helper()
	transport := dockerTransport(func(req *http.Request) (*http.Response, error) {
		if transportErr != nil {
			return nil, transportErr
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(response)),
			Request:    req,
		}, nil
	})
	client, err := dockerclient.New(
		dockerclient.WithHost("tcp://docker.test:2375"),
		dockerclient.WithAPIVersion("1.56"),
		dockerclient.WithHTTPClient(&http.Client{Transport: transport}),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.T().Cleanup(func() { _ = client.Close() })
	return &RuntimeService{client: client}
}

func validContainerSpec() domain.DeploymentSpec {
	return domain.DeploymentSpec{
		ProviderType:        domain.ModelProviderTypeHuggingFace,
		ProviderHuggingFace: &domain.HuggingFaceProvider{Repo: "org/model"},
	}
}

// Техника: разбиение на классы эквивалентности; случай успешного ответа Docker при создании контейнера.
func TestCreateContainerReturnsDockerID(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		runtime := runtimeWithDockerResponse(allureCtx, "{\"Id\":\"container-1\",\"Warnings\":[]}", nil)
		id, err := runtime.CreateContainer(t.Context(), "image", "model", validContainerSpec())
		if err != nil || id != "container-1" {
			allureCtx.Fatalf("id=%q err=%v", id, err)
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай ошибки Docker при создании контейнера.
func TestCreateContainerReportsDockerError(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		failure := errors.New("docker unavailable")
		runtime := runtimeWithDockerResponse(allureCtx, "", failure)
		id, err := runtime.CreateContainer(t.Context(), "image", "model", validContainerSpec())
		if id != "" || !errors.Is(err, failure) {
			allureCtx.Fatalf("id=%q err=%v", id, err)
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай найденного контейнера.
func TestGetContainerReturnsInspection(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		runtime := runtimeWithDockerResponse(allureCtx, "{\"Id\":\"container-1\",\"State\":{\"Status\":\"running\"}}", nil)
		info, err := runtime.GetContainer(t.Context(), "container-1")
		if err != nil || info.Container.ID != "container-1" || info.Container.State == nil || info.Container.State.Status != "running" {
			allureCtx.Fatalf("info=%+v err=%v", info, err)
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай ошибки Docker при чтении контейнера.
func TestGetContainerReportsDockerError(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		failure := errors.New("docker unavailable")
		runtime := runtimeWithDockerResponse(allureCtx, "", failure)
		_, err := runtime.GetContainer(t.Context(), "container-1")
		if !errors.Is(err, failure) {
			allureCtx.Fatalf("err=%v", err)
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай непустого списка контейнеров.
func TestListServiceContainersReturnsItems(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		runtime := runtimeWithDockerResponse(allureCtx, "[{\"Id\":\"container-1\"}]", nil)
		list, err := runtime.ListServiceContainers(t.Context(), "demo")
		if err != nil || len(list.Items) != 1 || list.Items[0].ID != "container-1" {
			allureCtx.Fatalf("items=%+v err=%v", list.Items, err)
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай ошибки Docker при чтении списка.
func TestListServiceContainersReportsDockerError(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		failure := errors.New("docker unavailable")
		runtime := runtimeWithDockerResponse(allureCtx, "", failure)
		_, err := runtime.ListServiceContainers(t.Context(), "demo")
		if !errors.Is(err, failure) {
			allureCtx.Fatalf("err=%v", err)
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай успешного запуска контейнера.
func TestStartContainerSucceeds(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		runtime := runtimeWithDockerResponse(allureCtx, "{}", nil)
		if err := runtime.StartContainer(t.Context(), "container-1"); err != nil {
			allureCtx.Fatal(err)
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай ошибки Docker при запуске контейнера.
func TestStartContainerReportsDockerError(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		failure := errors.New("docker unavailable")
		runtime := runtimeWithDockerResponse(allureCtx, "", failure)
		if err := runtime.StartContainer(t.Context(), "container-1"); !errors.Is(err, failure) {
			allureCtx.Fatalf("err=%v", err)
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай успешной остановки контейнера.
func TestStopContainerSucceeds(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		runtime := runtimeWithDockerResponse(allureCtx, "{}", nil)
		if err := runtime.StopContainer(t.Context(), "container-1"); err != nil {
			allureCtx.Fatal(err)
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай ошибки Docker при остановке контейнера.
func TestStopContainerReportsDockerError(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		failure := errors.New("docker unavailable")
		runtime := runtimeWithDockerResponse(allureCtx, "", failure)
		if err := runtime.StopContainer(t.Context(), "container-1"); !errors.Is(err, failure) {
			allureCtx.Fatalf("err=%v", err)
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай успешного удаления контейнера.
func TestRemoveContainerSucceeds(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		runtime := runtimeWithDockerResponse(allureCtx, "{}", nil)
		if err := runtime.RemoveContainer(t.Context(), "container-1"); err != nil {
			allureCtx.Fatal(err)
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай ошибки Docker при удалении контейнера.
func TestRemoveContainerReportsDockerError(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		failure := errors.New("docker unavailable")
		runtime := runtimeWithDockerResponse(allureCtx, "", failure)
		if err := runtime.RemoveContainer(t.Context(), "container-1"); !errors.Is(err, failure) {
			allureCtx.Fatalf("err=%v", err)
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай уже доступного образа.
func TestEnsureImageUsesLocalImage(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		runtime := runtimeWithDockerResponse(allureCtx, "{\"Id\":\"image\"}", nil)
		if err := runtime.EnsureImage(t.Context(), "image"); err != nil {
			allureCtx.Fatal(err)
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай ошибки Docker при проверке образа.
func TestEnsureImageReportsDockerError(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		failure := errors.New("docker unavailable")
		runtime := runtimeWithDockerResponse(allureCtx, "", failure)
		if err := runtime.EnsureImage(t.Context(), "image"); !errors.Is(err, failure) {
			allureCtx.Fatalf("err=%v", err)
		}
	})
}
