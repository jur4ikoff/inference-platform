package manager

import (
	"context"
	"errors"
	"github.com/containerd/errdefs"
	"github.com/google/uuid"
	"github.com/jur4ikoff/inference-service/internal/config"
	"github.com/jur4ikoff/inference-service/internal/domain"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"io"
	"sync"
	"testing"
	"time"
)

func managerConfig() config.DeploymentManagerConfig {
	return config.DeploymentManagerConfig{SyncInterval: time.Hour, OperationTimeout: time.Second, ReconnectInterval: time.Millisecond}
}

// Техника: тестирование переходов состояний контейнера.
func TestReconcileActualContainerStates(t *testing.T) {
	for _, test := range []struct {
		name     string
		previous domain.Status
		state    *container.State
		failure  error
		want     domain.Status
	}{
		{"running", domain.StatusPending, &container.State{Running: true}, nil, domain.StatusStarted},
		{"created", domain.StatusPending, &container.State{Status: container.StateCreated}, nil, domain.StatusPending},
		{"exited", domain.StatusStarted, &container.State{Status: container.StateExited}, nil, domain.StatusStopped},
		{"restarting", domain.StatusStarted, &container.State{Running: true, Restarting: true}, nil, domain.StatusStopping},
		{"removing", domain.StatusStarted, &container.State{Status: container.StateRemoving}, nil, domain.StatusStopping},
		{"stop in progress", domain.StatusStopping, &container.State{Running: true}, nil, domain.StatusStopping},
		{"ready model", domain.StatusRunning, &container.State{Running: true}, nil, domain.StatusRunning},
		{"removed", domain.StatusStarted, nil, errdefs.ErrNotFound, domain.StatusStopped},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := newMemoryRepository(test.previous)
			runtime := &memoryRuntime{state: test.state, failure: test.failure}
			m, err := NewDeploymentManager(repo, runtime, nil, managerConfig())
			if err != nil {
				t.Fatal(err)
			}
			err = m.Reconcile(t.Context())
			if err != nil || repo.current().Status != test.want {
				t.Fatalf("status=%s error=%v, want %s", repo.current().Status, err, test.want)
			}
		})
	}
}

// Техника: разбиение на классы эквивалентности; случай ошибки Docker при сверке состояния.
func TestReconcileReportsDockerError(t *testing.T) {
	repo := newMemoryRepository(domain.StatusStarted)
	failure := errors.New("Docker unavailable")
	manager, err := NewDeploymentManager(repo, &memoryRuntime{failure: failure}, nil, managerConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Reconcile(t.Context()); !errors.Is(err, failure) {
		t.Fatalf("err=%v", err)
	}
	if repo.current().Status != domain.StatusStarted {
		t.Fatal("status changed after Docker error")
	}
}

// Техника: разбиение на классы эквивалентности; случай ответа Docker без состояния контейнера.
func TestReconcileRejectsMissingContainerState(t *testing.T) {
	repo := newMemoryRepository(domain.StatusStarted)
	manager, err := NewDeploymentManager(repo, &memoryRuntime{}, nil, managerConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Reconcile(t.Context()); err == nil {
		t.Fatal("missing state accepted")
	}
	if repo.current().Status != domain.StatusStarted {
		t.Fatal("status changed after missing state")
	}
}

// Техника: тестирование переходов состояний с учётом устаревших событий.
func TestStoppingSignalsAndStaleEvents(t *testing.T) {
	now := time.Now().UTC()
	for _, test := range []struct {
		name, signal, stopSignal string
		eventTime                time.Time
		want                     domain.Status
	}{
		{"SIGTERM", "15", "", now, domain.StatusStopping},
		{"SIGKILL", "SIGKILL", "", now, domain.StatusStopping},
		{"custom stop signal", "SIGUSR1", "SIGUSR1", now, domain.StatusStopping},
		{"numeric custom stop signal", "10", "SIGUSR1", now, domain.StatusStopping},
		{"non-terminating signal", "SIGUSR1", "", now, domain.StatusStarted},
		{"old termination event", "15", "", now.Add(-time.Hour), domain.StatusStarted},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := newMemoryRepository(domain.StatusStarted)
			runtime := &memoryRuntime{state: &container.State{Running: true, StartedAt: now.Add(-time.Minute).Format(time.RFC3339Nano)}, stopSignal: test.stopSignal}
			m, _ := NewDeploymentManager(repo, runtime, nil, managerConfig())
			event := containerEvent(events.ActionKill)
			event.Actor.Attributes = map[string]string{"signal": test.signal}
			event.TimeNano = test.eventTime.UnixNano()
			if err := m.HandleEvent(t.Context(), event); err != nil {
				t.Fatal(err)
			}
			if got := repo.current().Status; got != test.want {
				t.Fatalf("got %s, want %s", got, test.want)
			}
		})
	}
}

// Техника: тестирование переходов состояний при задержанном событии.
func TestLateStopEventDoesNotOverwriteRestart(t *testing.T) {
	repo := newMemoryRepository(domain.StatusStopped)
	runtime := &memoryRuntime{state: &container.State{Running: true}}
	m, _ := NewDeploymentManager(repo, runtime, nil, managerConfig())
	if err := m.HandleEvent(t.Context(), containerEvent(events.ActionStop)); err != nil {
		t.Fatal(err)
	}
	if repo.current().Status != domain.StatusStarted {
		t.Fatal("manager must inspect the current state before applying a delayed stop event")
	}
}

// Техника: разбиение на классы эквивалентности; случай отсутствующего deployment.
func TestUnknownDeploymentIsIgnored(t *testing.T) {
	repo := newMemoryRepository(domain.StatusPending)
	repo.getErr = domain.ErrDeploymentNotFound
	runtime := &memoryRuntime{failure: errors.New("must not inspect")}
	m, _ := NewDeploymentManager(repo, runtime, nil, managerConfig())
	if err := m.HandleEvent(t.Context(), containerEvent(events.ActionStart)); err != nil {
		t.Fatal(err)
	}
	if repo.current().Status != domain.StatusPending {
		t.Fatal("an unknown deployment was updated")
	}
}

// Техника: тестирование переходов состояний по последовательности событий контейнера.
func TestRunUpdatesDeploymentStatusesFromContainerEvents(t *testing.T) {
	repo := newMemoryRepository(domain.StatusPending)
	inspected := make(chan struct{}, 8)
	runtime := &memoryRuntime{state: &container.State{Status: container.StateCreated}, inspected: inspected}
	messages := make(chan events.Message)
	streamErrors := make(chan error)
	connected := make(chan string, 4)
	runtime.watch = func(ctx context.Context, since string) client.EventsResult {
		connected <- since
		return client.EventsResult{Messages: messages, Err: streamErrors}
	}
	cfg := managerConfig()
	cfg.OperationTimeout = time.Hour
	m, _ := NewDeploymentManager(repo, runtime, nil, cfg)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx) }()
	receive(t, connected)
	receive(t, inspected)

	start := containerEvent(events.ActionStart)
	start.TimeNano = time.Now().Add(time.Second).UnixNano()
	runtime.setState(&container.State{Running: true})
	send(t, messages, start)
	if got := receive(t, repo.updates); got != domain.StatusStarted {
		t.Fatalf("got %s, want STARTED", got)
	}
	kill := containerEvent(events.ActionKill)
	kill.Actor.Attributes = map[string]string{"signal": "15"}
	send(t, messages, kill)
	if got := receive(t, repo.updates); got != domain.StatusStopping {
		t.Fatalf("got %s, want STOPPING", got)
	}
	runtime.setState(&container.State{Status: container.StateExited})
	send(t, messages, containerEvent(events.ActionDie))
	if got := receive(t, repo.updates); got != domain.StatusStopped {
		t.Fatalf("got %s, want STOPPED", got)
	}
	cancel()
	if err := receive(t, done); err != nil {
		t.Fatal(err)
	}
}

// Техника: тестирование переходов состояний; переход подключения после разрыва потока событий.
func TestRunReconnectsFromLastEvent(t *testing.T) {
	repo := newMemoryRepository(domain.StatusPending)
	inspected := make(chan struct{}, 2)
	runtime := &memoryRuntime{state: &container.State{Status: container.StateCreated}, inspected: inspected}
	messages := make(chan events.Message)
	streamErrors := make(chan error)
	connected := make(chan string, 4)
	runtime.watch = func(_ context.Context, since string) client.EventsResult {
		connected <- since
		return client.EventsResult{Messages: messages, Err: streamErrors}
	}
	cfg := managerConfig()
	cfg.OperationTimeout = time.Hour
	manager, err := NewDeploymentManager(repo, runtime, nil, cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- manager.Run(ctx) }()
	receive(t, connected)
	receive(t, inspected)
	event := containerEvent(events.ActionStart)
	event.TimeNano = time.Now().Add(time.Second).UnixNano()
	runtime.setState(&container.State{Running: true})
	send(t, messages, event)
	if got := receive(t, repo.updates); got != domain.StatusStarted {
		t.Fatalf("status=%s", got)
	}
	send[error](t, streamErrors, io.EOF)
	if cursor := receive(t, connected); cursor != eventTime(event).Format(time.RFC3339Nano) {
		t.Fatalf("reconnect cursor=%s", cursor)
	}
	cancel()
	if err := receive(t, done); err != nil {
		t.Fatal(err)
	}
}

// Техника: разбиение на классы эквивалентности; случай блокированного подключения при отмене.
func TestBlockedSubscriptionIsCanceled(t *testing.T) {
	repo := newMemoryRepository(domain.StatusPending)
	watchStarted := make(chan struct{})
	watchCanceled := make(chan struct{})
	runtime := &memoryRuntime{watch: func(ctx context.Context, _ string) client.EventsResult {
		close(watchStarted)
		<-ctx.Done()
		close(watchCanceled)
		return client.EventsResult{}
	}}
	cfg := managerConfig()
	cfg.OperationTimeout = time.Hour
	m, _ := NewDeploymentManager(repo, runtime, nil, cfg)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx) }()
	receive(t, watchStarted)
	cancel()
	if err := receive(t, done); err != nil {
		t.Fatal(err)
	}
	receive(t, watchCanceled)
}

func containerEvent(action events.Action) events.Message {
	return events.Message{Type: events.ContainerEventType, Action: action, Actor: events.Actor{ID: "docker-id"}}
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for manager")
		var zero T
		return zero
	}
}

func send[T any](t *testing.T, ch chan<- T, value T) {
	t.Helper()
	select {
	case ch <- value:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out sending a Docker event")
	}
}

type memoryRepository struct {
	mu      sync.Mutex
	state   domain.DeploymentState
	updates chan domain.Status
	getErr  error
}

func newMemoryRepository(status domain.Status) *memoryRepository {
	return &memoryRepository{state: domain.DeploymentState{ID: uuid.New(), ContainerID: "docker-id", Status: status, Project: "demo", DeploymentName: "qwen", Name: "qwen-abcde"}, updates: make(chan domain.Status, 16)}
}

func (r *memoryRepository) current() domain.DeploymentState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}

func (r *memoryRepository) ListDeploymentStates(context.Context) ([]domain.DeploymentState, error) {
	return []domain.DeploymentState{r.current()}, nil
}

func (r *memoryRepository) GetDeploymentState(context.Context, string) (domain.DeploymentState, error) {
	return r.current(), r.getErr
}

func (r *memoryRepository) UpdateDeploymentStatus(_ context.Context, previous domain.DeploymentState, next domain.Status) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state != previous {
		return false, nil
	}
	r.state.Status = next
	r.updates <- next
	return true, nil
}

type memoryRuntime struct {
	mu         sync.Mutex
	state      *container.State
	stopSignal string
	endpoint   string
	failure    error
	inspected  chan struct{}
	watch      func(context.Context, string) client.EventsResult
}

func (r *memoryRuntime) setState(state *container.State) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = state
}

func (r *memoryRuntime) GetContainer(context.Context, string) (client.ContainerInspectResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.inspected != nil {
		select {
		case r.inspected <- struct{}{}:
		default:
		}
	}
	return client.ContainerInspectResult{Container: container.InspectResponse{State: r.state, Config: &container.Config{StopSignal: r.stopSignal}}}, r.failure
}

func (r *memoryRuntime) WatchContainerEvents(ctx context.Context, since string) client.EventsResult {
	return r.watch(ctx, since)
}

func (r *memoryRuntime) ContainerEndpoint(client.ContainerInspectResult) (string, error) {
	if r.endpoint != "" {
		return r.endpoint, nil
	}
	return "http://model:8080", nil
}

// Техника: тестирование переходов состояний при отмене.
func TestManagerCancellationKeepsRunningDeployment(t *testing.T) {
	repo := newMemoryRepository(domain.StatusStarted)
	inspected := make(chan struct{}, 1)
	runtime := &memoryRuntime{state: &container.State{Running: true}, inspected: inspected}
	runtime.watch = func(ctx context.Context, _ string) client.EventsResult {
		return client.EventsResult{Messages: make(chan events.Message), Err: make(chan error)}
	}
	m, err := NewDeploymentManager(repo, runtime, nil, managerConfig())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx) }()
	receive(t, inspected)
	cancel()
	if err := receive(t, done); err != nil {
		t.Fatal(err)
	}
	if repo.current().Status != domain.StatusStarted || !runtime.state.Running {
		t.Fatal("manager cancellation changed a running deployment")
	}
}

// Техника: разбиение на классы эквивалентности; случай успешного чтения состояний.
func TestManagerInitPrunesRoutes(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMockManagerRepository(ctrl)
	routes := NewMockRouteRegistry(ctrl)
	repo.EXPECT().ListDeploymentStates(gomock.Any()).Return(nil, nil)
	routes.EXPECT().Prune([]domain.DeploymentState(nil))
	svc, err := NewDeploymentManager(repo, NewMockManagerRuntime(ctrl), routes, managerConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Init(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// Техника: разбиение на классы эквивалентности; случай ошибки зависимости при чтении состояний.
func TestManagerInitReportsRepositoryError(t *testing.T) {
	failure := errors.New("database unavailable")
	ctrl := gomock.NewController(t)
	repo := NewMockManagerRepository(ctrl)
	repo.EXPECT().ListDeploymentStates(gomock.Any()).Return(nil, failure)
	svc, err := NewDeploymentManager(repo, NewMockManagerRuntime(ctrl), NewMockRouteRegistry(ctrl), managerConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Init(t.Context()); !errors.Is(err, failure) {
		t.Fatalf("err=%v", err)
	}
}

var _ DeploymentRepository = (*MockManagerRepository)(nil)
var _ Runtime = (*MockManagerRuntime)(nil)
var _ RouteRegistry = (*MockRouteRegistry)(nil)
