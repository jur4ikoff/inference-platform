package manager

import (
	"context"
	"errors"
	"fmt"
)

// Shutdown is called after Run has stopped and incoming requests have drained.
func (m *DeploymentManager) Shutdown(ctx context.Context) error {
	var failures []error
	if err := m.runtime.StopModelContainers(ctx); err != nil {
		failures = append(failures, err)
	}
	if err := m.Reconcile(ctx); err != nil {
		failures = append(failures, fmt.Errorf("reconcile deployments after shutdown: %w", err))
	}
	return errors.Join(failures...)
}
