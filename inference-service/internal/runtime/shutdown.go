package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/containerd/errdefs"
	"github.com/jur4ikoff/inference-service/internal/modelgroup"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/rs/zerolog/log"
)

func (d *RuntimeService) StopModelContainers(ctx context.Context) error {
	filters := make(client.Filters).
		Add("label", "com.docker.compose.project="+modelgroup.Project).
		Add("status", string(container.StateRunning), string(container.StateRestarting), string(container.StatePaused))
	models, err := d.client.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: filters})
	if err != nil {
		return fmt.Errorf("list model containers for shutdown: %w", err)
	}
	log.Ctx(ctx).Info().Int("count", len(models.Items)).Msg("stopping model containers")
	failures := make(chan error, len(models.Items))
	var stopped sync.WaitGroup
	for _, model := range models.Items {
		stopped.Add(1)
		go func() {
			defer stopped.Done()
			if model.State == container.StatePaused {
				if _, err := d.client.ContainerUnpause(ctx, model.ID, client.ContainerUnpauseOptions{}); err != nil && !errdefs.IsNotModified(err) && !errdefs.IsNotFound(err) {
					failures <- fmt.Errorf("unpause model container %s for shutdown: %w", model.ID, err)
					return
				}
			}
			timeout := 5
			// Docker sends SIGKILL if the container does not exit within this timeout.
			_, err := d.client.ContainerStop(ctx, model.ID, client.ContainerStopOptions{Timeout: &timeout})
			if err != nil && !errdefs.IsNotFound(err) && !errdefs.IsNotModified(err) {
				failures <- fmt.Errorf("stop model container %s: %w", model.ID, err)
				return
			}
			log.Ctx(ctx).Info().Str("container_id", model.ID).Msg("model container stopped")
		}()
	}
	stopped.Wait()
	close(failures)
	var result []error
	for failure := range failures {
		result = append(result, failure)
	}
	return errors.Join(result...)
}
