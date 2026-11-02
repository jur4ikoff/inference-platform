package manager

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/google/uuid"
	"github.com/jur4ikoff/inference-service/internal/config"
	"github.com/jur4ikoff/inference-service/internal/domain"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
	"github.com/rs/zerolog/log"
)

type DeploymentRepository interface {
	ListDeploymentStates(context.Context) ([]domain.DeploymentState, error)
	GetDeploymentState(context.Context, string) (domain.DeploymentState, error)
	UpdateDeploymentStatus(context.Context, domain.DeploymentState, domain.Status) (bool, error)
}

type Runtime interface {
	GetContainer(context.Context, string) (client.ContainerInspectResult, error)
	WatchContainerEvents(context.Context, string) client.EventsResult
	ContainerEndpoint(client.ContainerInspectResult) (string, error)
	StopModelContainers(context.Context) error
}

type RouteRegistry interface {
	Upsert(domain.DeploymentState, string) error
	Remove(uuid.UUID)
	Prune([]domain.DeploymentState)
}

type DeploymentManager struct {
	repo    DeploymentRepository
	runtime Runtime
	config  config.DeploymentManagerConfig
	routes  RouteRegistry
}

func NewDeploymentManager(repo DeploymentRepository, runtime Runtime, routes RouteRegistry, cfg config.DeploymentManagerConfig) (*DeploymentManager, error) {
	if repo == nil || runtime == nil {
		return nil, errors.New("deployment manager requires a repository and runtime")
	}
	if cfg.SyncInterval <= 0 || cfg.OperationTimeout <= 0 || cfg.ReconnectInterval <= 0 {
		return nil, errors.New("deployment manager intervals and operation timeout must be positive")
	}
	return &DeploymentManager{repo: repo, runtime: runtime, routes: routes, config: cfg}, nil
}

func (m *DeploymentManager) Init(ctx context.Context) error {
	count, err := m.reconcile(ctx)
	if err != nil {
		return err
	}
	log.Ctx(ctx).Info().Int("deployments", count).Msg("initial deployment reconciliation completed")
	return nil
}

func (m *DeploymentManager) Run(ctx context.Context) error {
	since := time.Now().UTC()
	for ctx.Err() == nil {
		err := m.watch(ctx, &since)
		if ctx.Err() != nil {
			return nil
		}
		log.Ctx(ctx).Warn().Err(err).Msg("Docker event stream interrupted; reconnecting")
		timer := time.NewTimer(m.config.ReconnectInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
	return nil
}

func (m *DeploymentManager) watch(ctx context.Context, since *time.Time) error {
	streamCtx, cancelStream := context.WithCancel(ctx)
	defer cancelStream()
	connected := make(chan client.EventsResult, 1)
	cursor := since.Format(time.RFC3339Nano)
	go func() {
		connected <- m.runtime.WatchContainerEvents(streamCtx, cursor)
	}()
	timer := time.NewTimer(m.config.OperationTimeout)
	defer timer.Stop()
	var stream client.EventsResult
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return errors.New("timed out connecting to Docker events")
	case stream = <-connected:
	}
	timer.Stop()
	if stream.Messages == nil && stream.Err == nil {
		return errors.New("Docker returned an empty event stream")
	}
	m.reconcileAndLog(ctx)
	ticker := time.NewTicker(m.config.SyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err, ok := <-stream.Err:
			if !ok || err == nil {
				return io.EOF
			}
			return err
		case event, ok := <-stream.Messages:
			if !ok {
				return io.EOF
			}
			if err := m.HandleEvent(ctx, event); err != nil {
				return fmt.Errorf("handle Docker event: %w", err)
			}
			if timestamp := eventTime(event); timestamp.After(*since) {
				*since = timestamp
			}
		case <-ticker.C:
			m.reconcileAndLog(ctx)
		}
	}
}

func (m *DeploymentManager) reconcileAndLog(ctx context.Context) {
	if err := m.Reconcile(ctx); err != nil && ctx.Err() == nil {
		log.Ctx(ctx).Error().Err(err).Msg("failed to reconcile deployment statuses")
	}
}

func (m *DeploymentManager) Reconcile(ctx context.Context) error {
	_, err := m.reconcile(ctx)
	return err
}

func (m *DeploymentManager) reconcile(ctx context.Context) (int, error) {
	listCtx, cancel := context.WithTimeout(ctx, m.config.OperationTimeout)
	states, err := m.repo.ListDeploymentStates(listCtx)
	cancel()
	if err != nil {
		return 0, fmt.Errorf("list deployments: %w", err)
	}
	if m.routes != nil {
		m.routes.Prune(states)
	}
	var failures []error
	for _, state := range states {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		checkCtx, cancel := context.WithTimeout(ctx, m.config.OperationTimeout)
		err := m.reconcileContainer(checkCtx, state, nil)
		cancel()
		if err != nil {
			failures = append(failures, fmt.Errorf("deployment %s: %w", state.ID, err))
		}
	}
	return len(states), errors.Join(failures...)
}

func (m *DeploymentManager) HandleEvent(ctx context.Context, event events.Message) error {
	if event.Type != events.ContainerEventType || event.Actor.ID == "" {
		return nil
	}
	switch event.Action {
	case events.ActionStart, events.ActionRestart, events.ActionUnPause, events.ActionKill,
		events.ActionDie, events.ActionStop, events.ActionDestroy:
	default:
		return nil
	}
	checkCtx, cancel := context.WithTimeout(ctx, m.config.OperationTimeout)
	defer cancel()
	state, err := m.repo.GetDeploymentState(checkCtx, event.Actor.ID)
	if errors.Is(err, domain.ErrDeploymentNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return m.reconcileContainer(checkCtx, state, &event)
}

func (m *DeploymentManager) reconcileContainer(ctx context.Context, previous domain.DeploymentState, event *events.Message) error {
	info, err := m.runtime.GetContainer(ctx, previous.ContainerID)
	next := previous.Status
	switch {
	case errdefs.IsNotFound(err):
		next = domain.StatusStopped
	case err != nil:
		return fmt.Errorf("inspect container: %w", err)
	case info.Container.State == nil:
		return errors.New("Docker returned no container state")
	default:
		state := info.Container.State
		switch {
		case state.Restarting || state.Status == container.StateRemoving:
			next = domain.StatusStopping
		case state.Running:
			if event != nil && event.Action == events.ActionKill {
				if !isStopSignal(event.Actor.Attributes["signal"], info.Container.Config) {
					return nil
				}
				// Replayed events from an earlier run must not stop a restarted container.
				if startedAt, err := time.Parse(time.RFC3339Nano, state.StartedAt); err == nil && !eventTime(*event).IsZero() && eventTime(*event).Before(startedAt) {
					return nil
				}
				next = domain.StatusStopping
			} else if event != nil && (event.Action == events.ActionStart || event.Action == events.ActionRestart) {
				next = domain.StatusStarted
			} else if previous.Status != domain.StatusStopping && previous.Status != domain.StatusRunning {
				next = domain.StatusStarted
			}
		case state.Status == container.StateExited || state.Status == container.StateDead || state.Dead:
			next = domain.StatusStopped
		}
	}
	active := err == nil && info.Container.State != nil && info.Container.State.Running &&
		!info.Container.State.Paused && !info.Container.State.Restarting &&
		(next == domain.StatusStarted || next == domain.StatusRunning)
	if !active && m.routes != nil {
		m.routes.Remove(previous.ID)
	}
	if next != previous.Status {
		updated, err := m.repo.UpdateDeploymentStatus(ctx, previous, next)
		if err != nil {
			return err
		}
		if !updated {
			return nil
		}
		log.Ctx(ctx).Info().Str("deployment_id", previous.ID.String()).
			Str("container_id", previous.ContainerID).Str("from", string(previous.Status)).
			Str("to", string(next)).Msg("deployment status changed")
	}
	if active && m.routes != nil {
		endpoint, err := m.runtime.ContainerEndpoint(info)
		if err != nil {
			return fmt.Errorf("resolve container endpoint: %w", err)
		}
		previous.Status = next
		return m.routes.Upsert(previous, endpoint)
	}
	return nil
}

func isStopSignal(signal string, cfg *container.Config) bool {
	normalize := func(value string) string {
		value = strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(value)), "SIG")
		// Docker reports numeric Linux signals, while image settings often use names.
		if number, ok := map[string]string{
			"HUP": "1", "INT": "2", "QUIT": "3", "ILL": "4", "TRAP": "5",
			"ABRT": "6", "IOT": "6", "BUS": "7", "FPE": "8", "KILL": "9",
			"USR1": "10", "SEGV": "11", "USR2": "12", "PIPE": "13", "ALRM": "14",
			"TERM": "15", "STKFLT": "16", "CHLD": "17", "CONT": "18", "STOP": "19",
			"TSTP": "20", "TTIN": "21", "TTOU": "22", "URG": "23", "XCPU": "24",
			"XFSZ": "25", "VTALRM": "26", "PROF": "27", "WINCH": "28", "IO": "29",
			"POLL": "29", "PWR": "30", "SYS": "31",
		}[value]; ok {
			return number
		}
		return value
	}
	signal = normalize(signal)
	switch signal {
	case "15", "2", "9":
		return true
	}
	return signal != "" && cfg != nil && cfg.StopSignal != "" && signal == normalize(cfg.StopSignal)
}

func eventTime(event events.Message) time.Time {
	if event.TimeNano != 0 {
		return time.Unix(0, event.TimeNano)
	}
	if event.Time != 0 {
		return time.Unix(event.Time, 0)
	}
	return time.Time{}
}
