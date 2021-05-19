package mainutil

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/chronos-tachyon/roxy/internal/misc"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
)

type MultiServer struct {
	wg           sync.WaitGroup
	runList      []func()
	reloadList   []func() error
	shutdownList []func(bool) error
	exitList     []func() error

	mu            sync.Mutex
	shutdownCh    chan struct{}
	alreadyTermed bool
	alreadyClosed bool
}

func (m *MultiServer) Go(fn func()) {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		fn()
	}()
}

func (m *MultiServer) OnRun(fn func()) {
	m.runList = append(m.runList, fn)
}

func (m *MultiServer) OnReload(fn func() error) {
	m.reloadList = append(m.reloadList, fn)
}

func (m *MultiServer) OnShutdown(fn func(bool) error) {
	m.shutdownList = append(m.shutdownList, fn)
}

func (m *MultiServer) OnExit(fn func() error) {
	m.exitList = append(m.exitList, fn)
}

func (m *MultiServer) AddHTTPServer(name string, server *http.Server, listen net.Listener) {
	if server == nil {
		panic(errors.New("*http.Server is nil"))
	}
	if listen == nil {
		panic(errors.New("net.Listener is nil"))
	}
	ctx := RootContext()
	m.OnRun(func() {
		err := server.Serve(listen)
		m.closeShutdownCh()
		if isRealShutdownError(err) {
			log.Logger.Error().
				Str("subsystem", name).
				Err(err).
				Msg("failed to Serve")
		}
	})
	m.OnShutdown(func(alreadyTermed bool) error {
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

func (m *MultiServer) AddGRPCServer(name string, server *grpc.Server, listen net.Listener) {
	if server == nil {
		panic(errors.New("*grpc.Server is nil"))
	}
	if listen == nil {
		panic(errors.New("net.Listener is nil"))
	}
	m.OnRun(func() {
		err := server.Serve(listen)
		m.closeShutdownCh()
		if isRealShutdownError(err) {
			log.Logger.Error().
				Str("subsystem", name).
				Err(err).
				Msg("failed to Serve")
		}
	})
	m.OnShutdown(func(alreadyTermed bool) error {
		if alreadyTermed {
			server.Stop()
		} else {
			go server.GracefulStop()
		}
		return nil
	})
}

func (m *MultiServer) Run() {
	m.shutdownCh = make(chan struct{})
	m.alreadyTermed = false
	m.alreadyClosed = false

	sdNotify("READY=1")

	for _, fn := range m.runList {
		m.Go(fn)
	}

	ctx := RootContext()
	doneCh := ctx.Done()
	exitCh := make(chan struct{})

	go func() {
		select {
		case <-doneCh:
			return

		case <-m.shutdownCh:
			// pass
		}

		_ = m.Shutdown(true)

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

		_ = m.Shutdown(false)
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
					_ = m.Shutdown(false)
				case syscall.SIGHUP:
					_ = m.Reload()
				}
			}
		}
	}()

	go func() {
		m.wg.Wait()
		close(exitCh)
	}()

	<-exitCh
}

func (m *MultiServer) Reload() error {
	var errs multierror.Error
	sdNotify("RELOADING=1")
	for _, fn := range m.reloadList {
		if err := fn(); err != nil {
			errs.Errors = append(errs.Errors, err)
		}
	}
	sdNotify("READY=1")
	return misc.ErrorOrNil(errs)
}

func (m *MultiServer) Shutdown(graceful bool) error {
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

	var wg sync.WaitGroup
	errCh := make(chan error)

	for index := uint(len(m.shutdownList)); index > 0; index-- {
		fn := m.shutdownList[index-1]
		wg.Add(1)
		go func(fn func(bool) error) {
			defer wg.Done()
			if err := fn(alreadyTermed); err != nil {
				errCh <- err
			}
		}(fn)
	}

	go func() {
		wg.Wait()
		for index := uint(len(m.exitList)); index > 0; index-- {
			fn := m.exitList[index-1]
			if err := fn(); err != nil {
				errCh <- err
			}
		}
		close(errCh)
	}()

	var errs multierror.Error
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
