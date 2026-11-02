package runtime

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"

	"github.com/containerd/errdefs"
	"github.com/jur4ikoff/inference-service/internal/config"
	"github.com/jur4ikoff/inference-service/internal/domain"
	"github.com/jur4ikoff/inference-service/internal/modelgroup"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	dockerClient "github.com/moby/moby/client"
	"github.com/rs/zerolog/log"
)

type RuntimeService struct {
	runtimeConfig config.RuntimeServiceConfig
	client        *dockerClient.Client
}

func NewRuntimeService(runtimeConfig config.RuntimeServiceConfig) (*RuntimeService, error) {
	cli, err := dockerClient.New(dockerClient.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &RuntimeService{runtimeConfig: runtimeConfig,
		client: cli}, nil
}

func (d *RuntimeService) Close() error {
	return d.client.Close()
}

func (d *RuntimeService) GetContainer(ctx context.Context, id string) (dockerClient.ContainerInspectResult, error) {
	return d.client.ContainerInspect(ctx, id, dockerClient.ContainerInspectOptions{})
}

func (d *RuntimeService) ContainerEndpoint(info dockerClient.ContainerInspectResult) (string, error) {
	if d.runtimeConfig.DockerNetwork != "" {
		name := strings.TrimPrefix(info.Container.Name, "/")
		if name == "" {
			return "", fmt.Errorf("container has no name")
		}
		return (&url.URL{Scheme: "http", Host: net.JoinHostPort(name, "8080")}).String(), nil
	}
	if info.Container.NetworkSettings != nil {
		for _, binding := range info.Container.NetworkSettings.Ports[network.MustParsePort("8080/tcp")] {
			if binding.HostPort != "" {
				host := binding.HostIP.String()
				if !binding.HostIP.IsValid() || binding.HostIP.IsUnspecified() {
					host = "127.0.0.1"
				}
				return (&url.URL{Scheme: "http", Host: net.JoinHostPort(host, binding.HostPort)}).String(), nil
			}
		}
	}
	return "", fmt.Errorf("container has no published HTTP port and DOCKER_NETWORK is empty")
}

func (d *RuntimeService) WatchContainerEvents(ctx context.Context, since string) dockerClient.EventsResult {
	filters := make(dockerClient.Filters).
		Add("type", string(events.ContainerEventType)).
		Add("label", "com.docker.compose.project="+modelgroup.Project).
		Add("event", "start", "kill", "die", "stop", "restart", "destroy", "unpause")
	return d.client.Events(ctx, dockerClient.EventsListOptions{Since: since, Filters: filters})
}

func (d *RuntimeService) CreateContainer(ctx context.Context, image, containerName string, spec domain.DeploymentSpec) (string, error) {
	options, err := d.makeCreateOptions(image, containerName, spec)
	if err != nil {
		return "", fmt.Errorf("make create options: %w", err)
	}

	result, err := d.client.ContainerCreate(ctx, options)
	if err != nil {
		return "", fmt.Errorf("create container by docker cli: %w", err)
	}

	for _, warning := range result.Warnings {
		log.Ctx(ctx).Warn().Msgf("warn via create container %s", warning)
	}

	return result.ID, nil
}

func (d *RuntimeService) makeCreateOptions(image, containerName string, spec domain.DeploymentSpec) (dockerClient.ContainerCreateOptions, error) {
	if spec.ProviderType != domain.ModelProviderTypeHuggingFace {
		return dockerClient.ContainerCreateOptions{}, fmt.Errorf("provider with type: %s not supported", spec.ProviderType)
	}
	if spec.ProviderHuggingFace == nil {
		return dockerClient.ContainerCreateOptions{}, fmt.Errorf("provider settings dont set")
	}
	port := network.MustParsePort("8080/tcp")
	var bindings network.PortMap
	if d.runtimeConfig.DockerNetwork == "" {
		bindings = network.PortMap{port: {{HostIP: netip.MustParseAddr("127.0.0.1"), HostPort: d.runtimeConfig.PortHost}}}
	}

	return dockerClient.ContainerCreateOptions{
		Image: image,
		Name:  containerName,

		Config: &container.Config{
			Labels: modelgroup.Labels(containerName),
			Cmd: []string{
				"-hf", spec.ProviderHuggingFace.Repo,
				"--alias", spec.ProviderHuggingFace.Repo,
				"--host", "0.0.0.0",
				"--port", "8080",
				"--ctx-size", "2048",
			},
			Env: []string{
				"LLAMA_CACHE=/cache",
			},
			ExposedPorts: network.PortSet{
				port: {},
			},
		},
		HostConfig: &container.HostConfig{
			NetworkMode:  container.NetworkMode(d.runtimeConfig.DockerNetwork),
			PortBindings: bindings,
			Mounts: []mount.Mount{
				{
					Type:   mount.TypeVolume,
					Source: "llama-model-cache",
					Target: "/cache",
				},
			},
		},
	}, nil
}

func (d *RuntimeService) StartContainer(ctx context.Context, id string) error {
	_, err := d.client.ContainerStart(ctx, id, dockerClient.ContainerStartOptions{})
	return err
}

func (d *RuntimeService) StopContainer(ctx context.Context, id string) error {
	_, err := d.client.ContainerStop(ctx, id, dockerClient.ContainerStopOptions{})
	return err
}

func (d *RuntimeService) RemoveContainer(ctx context.Context, id string) error {
	_, err := d.client.ContainerRemove(ctx, id, dockerClient.ContainerRemoveOptions{})
	return err
}

func (d *RuntimeService) EnsureImage(ctx context.Context, image string) error {
	_, err := d.client.ImageInspect(ctx, image)
	if err != nil {
		if !errdefs.IsNotFound(err) {
			return fmt.Errorf("inspect Docker image: %w", err)
		}
		return d.PullImage(ctx, image)
	}
	log.Ctx(ctx).Info().Str("image", image).Msg("using local Docker image")
	return nil
}

func (d *RuntimeService) PullImage(ctx context.Context, image string) error {
	log.Ctx(ctx).Info().Msgf("pulling image %s", image)
	pull, err := d.client.ImagePull(ctx, image, dockerClient.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("pull Docker image: %w", err)
	}
	defer pull.Close()
	if err := pull.Wait(ctx); err != nil {
		return fmt.Errorf("download Docker image: %w", err)
	}
	return nil
}
