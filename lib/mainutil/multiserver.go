package mainutil

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	multierror "github.com/hashicorp/go-multierror"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/announcer"
	"github.com/chronos-tachyon/roxy/lib/membership"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

// HealthWatchID tracks a registered HealthWatchFunc callback.
type HealthWatchID uint32

// HealthWatchFunc is the callback type for observing changes in health status.
type HealthWatchFunc func(subsystemName string, isHealthy bool, isStopped bool)

// PreRunFunc is the callback type for OnPreRun hooks.
type PreRunFunc func(ctx context.Context) error

// RunFunc is the callback type for OnRun hooks.
type RunFunc func(ctx context.Context)

// ReloadFunc is the callback type for OnReload hooks.
type ReloadFunc func(ctx context.Context) error

// PreShutdownFunc is the callback type for OnPreShutdown hooks.
type PreShutdownFunc func(ctx context.Context) error

// ShutdownFunc is the callback type for OnShutdown hooks.
type ShutdownFunc func(ctx context.Context, alreadyTermed bool) error

// ExitFunc is the callback type for OnExit hooks.
type ExitFunc func(ctx context.Context) error

// MultiServer is a framework that tracks state for long-running servers.  It
// remembers which steps need to execute at which phase of the server's
// lifetime, then calls those steps as needed (with parallelism when
// appropriate).
//
// It is most useful for keeping track of multiple http.Server and grpc.Server
// instances running in parallel on different net.Listeners.  It also automates
// signal management and communication with Systemd.
type MultiServer struct {
	wg              sync.WaitGroup
	preRunList      []PreRunFunc
	runList         []RunFunc
	reloadList      []ReloadFunc
	preShutdownList []PreShutdownFunc
	shutdownList    []ShutdownFunc
	exitList        []ExitFunc

	mu            sync.Mutex
	shutdownCh    chan struct{}
	healthMap     map[string]bool
	watchMap      map[HealthWatchID]HealthWatchFunc
	lastWatchID   HealthWatchID
	isStopped     bool
	alreadyTermed bool
	alreadyClosed bool
}

// HealthServer returns an implementation of grpc_health_v1.HealthServer that
// uses the health status reported to this MultiServer.
func (m *MultiServer) HealthServer() grpc_health_v1.HealthServer {
	return healthServer{m: m}
}

// GetHealth retrieves the health status of the named subsystem.
func (m *MultiServer) GetHealth(subsystemName string) (isHealthy bool, found bool) {
	m.mu.Lock()
	isHealthy, found = m.healthMap[subsystemName]
	m.mu.Unlock()
	return
}

// SetHealth changes the health status for the named subsystem.  If the
// subsystem does not yet exist, it is automatically created.
func (m *MultiServer) SetHealth(subsystemName string, isHealthy bool) {
	m.mu.Lock()
	if m.isStopped {
		m.mu.Unlock()
		return
	}
	if m.healthMap == nil {
		m.healthMap = make(map[string]bool, 16)
	}
	oldIsHealthy, found := m.healthMap[subsystemName]
	if found && oldIsHealthy == isHealthy {
		m.mu.Unlock()
		return
	}
	m.healthMap[subsystemName] = isHealthy
	for _, fn := range m.watchMap {
		callWatchFunc(fn, subsystemName, isHealthy, false)
	}
	m.mu.Unlock()
}

// WatchHealth registers a callback that will receive a snapshot of the
// current health status plus a subscription to all future health status
// changes.
func (m *MultiServer) WatchHealth(fn HealthWatchFunc) HealthWatchID {
	m.mu.Lock()
	m.lastWatchID++
	id := m.lastWatchID
	if !m.isStopped {
		if m.watchMap == nil {
			m.watchMap = make(map[HealthWatchID]HealthWatchFunc, 4)
		}
		m.watchMap[id] = fn
	}
	for subsystemName, isHealthy := range m.healthMap {
		callWatchFunc(fn, subsystemName, isHealthy, m.isStopped)
	}
	m.mu.Unlock()

	return id
}

// CancelWatchHealth unregisters a callback.
func (m *MultiServer) CancelWatchHealth(id HealthWatchID) {
	m.mu.Lock()
	if m.watchMap != nil {
		delete(m.watchMap, id)
	}
	m.mu.Unlock()
}

// Go runs a function in a goroutine.  The Run method will not return until
// after fn has terminated.
func (m *MultiServer) Go(ctx context.Context, fn RunFunc) {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		fn(ctx)
	}()
}

// OnPreRun registers a function to execute when Run is called.  Each such
// function will be called serially before any OnRun hooks execute.  If any
// such function returns an error, Run will exit immediately with an error.
//
// OnPreRun hooks are called in FIFO (First In, First Out) order.
//
// OnPreRun MUST NOT be called after Run has been called.
func (m *MultiServer) OnPreRun(fn PreRunFunc) {
	m.preRunList = append(m.preRunList, fn)
}

// OnRun registers a function to execute in a goroutine when Run is called.
//
// OnRun hooks are called in FIFO (First In, First Out) order.
//
// OnRun MUST NOT be called after Run has been called.
func (m *MultiServer) OnRun(fn RunFunc) {
	m.runList = append(m.runList, fn)
}

// OnReload registers a function to execute when SIGHUP is received or when
// Reload is called.
//
// OnReload hooks are called in FIFO (First In, First Out) order.
//
// OnReload MUST NOT be called after Run has been called.
func (m *MultiServer) OnReload(fn ReloadFunc) {
	m.reloadList = append(m.reloadList, fn)
}

// OnPreShutdown registers a function to execute when SIGINT/SIGTERM are
// received or when Shutdown is called.  Each such function will be called
// serially before any OnShutdown hooks execute.
//
// OnPreShutdown hooks are called in LIFO (Last In, First Out) order.
//
// OnPreShutdown MUST NOT be called after Run has been called.
func (m *MultiServer) OnPreShutdown(fn PreShutdownFunc) {
	m.preShutdownList = append(m.preShutdownList, fn)
}

// OnShutdown registers a function to execute in a goroutine when
// SIGINT/SIGTERM are received or when Shutdown is called.
//
// The function will be called with a bool argument, alreadyTermed.  If true,
// then this is the second attempt to shut down the server, and the user is
// potentially getting impatient.  If false, a graceful shutdown should be
// attempted.
//
// OnShutdown hooks are called in LIFO (Last In, First Out) order.
//
// OnShutdown MUST NOT be called after Run has been called.
func (m *MultiServer) OnShutdown(fn ShutdownFunc) {
	m.shutdownList = append(m.shutdownList, fn)
}

// OnExit registers a function to execute just before Run returns.
//
// OnExit hooks are called in LIFO (Last In, First Out) order.
//
// OnExit MUST NOT be called after Run has been called.
func (m *MultiServer) OnExit(fn ExitFunc) {
	m.exitList = append(m.exitList, fn)
}

// AddHTTPServer registers an HTTP(S) server.  This will arrange for the Run
// method to invoke server.Serve(listen), and for Shutdown to invoke
// server.Shutdown(ctx) or server.Close(), as appropriate.
//
// AddHTTPServer MUST NOT be called after Run has been called.
func (m *MultiServer) AddHTTPServer(name string, server *http.Server, listen net.Listener) {
	if server == nil {
		panic(errors.New("*http.Server is nil"))
	}
	if listen == nil {
		panic(errors.New("net.Listener is nil"))
	}
	m.OnRun(func(ctx context.Context) {
		err := server.Serve(listen)
		m.closeShutdownCh()
		if isRealShutdownError(err) {
			log.Logger.Error().
				Str("subsystem", name).
				Err(err).
				Msg("failed to Serve")
		}
	})
	m.OnShutdown(func(ctx context.Context, alreadyTermed bool) error {
		var action string
		var err error
		if alreadyTermed {
			action = "Close"
			err = server.Close()
		} else {
			action = "Shutdown"
			err = server.Shutdown(ctx)
		}
		if isRealShutdownError(err) {
			log.Logger.Error().
				Str("subsystem", name).
				Err(err).
				Msg("failed to " + action)
			return err
		}
		return nil
	})
}

// AddGRPCServer registers a gRPC server.  This will arrange for the Run method
// to invoke server.Serve(listen), and for Shutdown to invoke
// server.GracefulStop() or server.Stop(), as appropriate.
//
// AddGRPCServer MUST NOT be called after Run has been called.
func (m *MultiServer) AddGRPCServer(name string, server *grpc.Server, listen net.Listener) {
	if server == nil {
		panic(errors.New("*grpc.Server is nil"))
	}
	if listen == nil {
		panic(errors.New("net.Listener is nil"))
	}
	m.OnRun(func(ctx context.Context) {
		err := server.Serve(listen)
		m.closeShutdownCh()
		if isRealShutdownError(err) {
			log.Logger.Error().
				Str("subsystem", name).
				Err(err).
				Msg("failed to Serve")
		}
	})
	m.OnShutdown(func(ctx context.Context, alreadyTermed bool) error {
		if alreadyTermed {
			server.Stop()
		} else {
			go server.GracefulStop()
		}
		return nil
	})
}

// AddAnnouncer registers an Announcer and the address to be announced.  This
// will arrange for invoke Announcer.Announce to run in an OnPreRun hook, for
// Announcer.Withdraw to run in an OnPreShutdown hook, and for Announcer.Close
// to run in an OnExit hook.
//
// AddAnnouncer MUST NOT be called after Run has been called.
func (m *MultiServer) AddAnnouncer(ann *announcer.Announcer, r *membership.Roxy) {
	m.OnPreRun(func(ctx context.Context) error {
		err := ann.Announce(ctx, r)
		if err != nil {
			log.Logger.Fatal().
				Err(err).
				Msg("Announcer.Announce failed")
		}
		return err
	})
	m.OnPreShutdown(func(ctx context.Context) error {
		err := ann.Withdraw(ctx)
		if err != nil {
			log.Logger.Error().
				Err(err).
				Msg("Announcer.Withdraw failed")
		}
		return err
	})
	m.OnExit(func(ctx context.Context) error {
		err := ann.Close()
		if err != nil {
			log.Logger.Error().
				Err(err).
				Msg("Announcer.Close failed")
		}
		return err
	})
}

// Run runs the MultiServer.  It does not return until all registered servers
// have been fully shut down, all goroutines have exited, and all OnExit hooks
// have completed.
func (m *MultiServer) Run(ctx context.Context) error {
	roxyutil.AssertNotNil(&ctx)

	m.shutdownCh = make(chan struct{})
	m.alreadyTermed = false
	m.alreadyClosed = false

	var errs multierror.Error
	for _, fn := range m.preRunList {
		if err := fn(ctx); err != nil {
			errs.Errors = append(errs.Errors, err)
		}
	}
	if errs.Errors != nil {
		close(m.shutdownCh)
		m.alreadyTermed = true
		m.alreadyClosed = true
		return misc.ErrorOrNil(errs)
	}

	sdNotify("READY=1")

	for _, fn := range m.runList {
		m.Go(ctx, fn)
	}

	doneCh := ctx.Done()
	exitCh := make(chan struct{})

	go func() {
		select {
		case <-doneCh:
			return

		case <-m.shutdownCh:
			// pass
		}

		_ = m.Shutdown(ctx, true)

		t := time.NewTimer(5 * time.Second)

		select {
		case <-doneCh:
			t.Stop()
			return

		case <-exitCh:
			t.Stop()
			return

		case <-t.C:
			// pass
		}

		_ = m.Shutdown(ctx, false)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		defer signal.Stop(sigCh)
		for {
			select {
			case <-doneCh:
				return

			case <-exitCh:
				return

			case sig := <-sigCh:
				log.Logger.Info().
					Str("sig", sig.String()).
					Msg("got signal")
				switch sig {
				case syscall.SIGINT:
					fallthrough
				case syscall.SIGTERM:
					_ = m.Shutdown(ctx, false)
				case syscall.SIGHUP:
					_ = m.Reload(ctx)
				}
			}
		}
	}()

	go func() {
		m.wg.Wait()
		close(exitCh)
	}()

	log.Logger.Info().
		Msg("Running")

	<-exitCh

	for index := uint(len(m.exitList)); index > 0; index-- {
		fn := m.exitList[index-1]
		if err := fn(ctx); err != nil {
			errs.Errors = append(errs.Errors, err)
		}
	}

	log.Logger.Info().
		Msg("Exit")

	return misc.ErrorOrNil(errs)
}

// Reload triggers a server reload.  It may be called from any thread.
func (m *MultiServer) Reload(ctx context.Context) error {
	roxyutil.AssertNotNil(&ctx)

	var errs multierror.Error
	sdNotify("RELOADING=1")
	for _, fn := range m.reloadList {
		if err := fn(ctx); err != nil {
			errs.Errors = append(errs.Errors, err)
		}
	}
	sdNotify("READY=1")
	return misc.ErrorOrNil(errs)
}

// Shutdown triggers a server shutdown.  It may be called from any thread.
//
// If graceful is true, then only graceful shutdown techniques will be
// considered.  If graceful is false, then forceful techniques will be
// considered.  Even if graceful is true, a forceful shutdown will be triggered
// if the graceful shutdown phase takes longer than 5 seconds.
func (m *MultiServer) Shutdown(ctx context.Context, graceful bool) error {
	roxyutil.AssertNotNil(&ctx)

	m.mu.Lock()
	alreadyTermed := m.alreadyTermed
	m.alreadyTermed = true
	m.mu.Unlock()

	if alreadyTermed && graceful {
		return nil
	}

	if alreadyTermed {
		log.Logger.Warn().
			Msg("forcing dirty shutdown")
		CancelRootContext()
	}

	sdNotify("STOPPING=1")

	m.mu.Lock()
	if !m.isStopped {
		m.isStopped = true
		for subsystemName := range m.healthMap {
			m.healthMap[subsystemName] = false
		}
		watchMap := m.watchMap
		m.watchMap = nil
		for _, fn := range watchMap {
			for subsystemName := range m.healthMap {
				callWatchFunc(fn, subsystemName, false, true)
			}
		}
	}
	m.mu.Unlock()

	var errs multierror.Error
	for index := uint(len(m.preShutdownList)); index > 0; index-- {
		fn := m.preShutdownList[index-1]
		if err := fn(ctx); err != nil {
			errs.Errors = append(errs.Errors, err)
		}
	}

	var wg sync.WaitGroup
	errCh := make(chan error)

	for index := uint(len(m.shutdownList)); index > 0; index-- {
		fn := m.shutdownList[index-1]
		wg.Add(1)
		go func(fn ShutdownFunc) {
			defer wg.Done()
			if err := fn(ctx, alreadyTermed); err != nil {
				errCh <- err
			}
		}(fn)
	}

	go func() {
		wg.Wait()
		close(errCh)
	}()

	for err := range errCh {
		errs.Errors = append(errs.Errors, err)
	}
	return misc.ErrorOrNil(errs)
}

func (m *MultiServer) closeShutdownCh() {
	m.mu.Lock()
	if !m.alreadyClosed {
		m.alreadyClosed = true
		close(m.shutdownCh)
	}
	m.mu.Unlock()
}

func isRealShutdownError(err error) bool {
	switch {
	case err == nil:
		return false

	case errors.Is(err, fs.ErrClosed):
		return false

	case errors.Is(err, net.ErrClosed):
		return false

	case errors.Is(err, http.ErrServerClosed):
		return false

	case errors.Is(err, context.Canceled):
		return false

	default:
		return true
	}
}

func callWatchFunc(fn HealthWatchFunc, subsystemName string, isHealthy bool, isStopped bool) {
	defer func() {
		panicValue := recover()
		if panicValue == nil {
			return
		}
		if err, ok := panicValue.(error); ok {
			log.Logger.Error().
				Err(err).
				Msg("panic in HealthWatchFunc")
			return
		}
		log.Logger.Error().
			Str("panicType", fmt.Sprintf("%T", panicValue)).
			Interface("panicValue", panicValue).
			Msg("panic in HealthWatchFunc")
	}()
	fn(subsystemName, isHealthy, isStopped)
}
