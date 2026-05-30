// Package core provides the standalone iPShadowT tunnel engine.
// It can be used as a library in other Go projects or as the foundation
// for CLI, GUI, mobile, or web applications.
//
// Basic usage:
//
//	engine, err := core.New(core.WithConfigFile("config.toml"))
//	if err != nil { log.Fatal(err) }
//	if err := engine.Start(); err != nil { log.Fatal(err) }
//	defer engine.Stop()
//
// Programmatic usage:
//
//	engine, err := core.New(
//	    core.WithMode(core.ModeClient),
//	    core.WithTransport(core.TransportReality),
//	    core.WithRemoteAddr("server.com:443"),
//	    core.WithPassword("my-secret"),
//	    core.WithSOCKS5("127.0.0.1:1080"),
//	    core.WithAntiDPI(true),
//	)
//	if err != nil { log.Fatal(err) }
//	engine.Start()
package core

import (
	"fmt"
	"sync"

	"github.com/iPmart/iPShadowT/internal/client"
	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/logger"
	"github.com/iPmart/iPShadowT/internal/server"
)

// Version is the engine version
const Version = "v1.0.0"

// Author is the project author
const Author = "iPmart Network (Ali Hassanzadeh)"

// ProjectURL is the project repository
const ProjectURL = "https://github.com/iPmart/iPShadowT"

// Mode represents the engine operating mode
type Mode string

const (
	ModeClient Mode = "client"
	ModeServer Mode = "server"
)

// TransportType represents available transports
type TransportType string

const (
	TransportTCPMux    TransportType = "tcpmux"
	TransportWSMux     TransportType = "wsmux"
	TransportH2Mux     TransportType = "h2mux"
	TransportGRPC      TransportType = "grpc"
	TransportReality   TransportType = "reality"
	TransportShadowTLS TransportType = "shadowtls"
	TransportQUIC      TransportType = "quic"
	TransportReverse   TransportType = "reverse"
)

// State represents the engine state
type State int

const (
	StateStopped State = iota
	StateStarting
	StateRunning
	StateStopping
	StateError
)

// Engine is the core iPShadowT tunnel engine
type Engine struct {
	cfg       *config.Config
	log       *logger.Logger
	state     State
	mu        sync.RWMutex
	server    *server.Server
	client    *client.Client
	events    *EventBus
	lastError error
}

// New creates a new engine with the given options
func New(opts ...Option) (*Engine, error) {
	// Apply options
	o := &Options{}
	for _, opt := range opts {
		opt(o)
	}

	// Build config
	cfg, err := o.buildConfig()
	if err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Create logger
	log := logger.New(cfg.LogLevel)

	return &Engine{
		cfg:    cfg,
		log:    log,
		state:  StateStopped,
		events: NewEventBus(),
	}, nil
}

// Start starts the tunnel engine
func (e *Engine) Start() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state == StateRunning {
		return fmt.Errorf("engine already running")
	}

	e.state = StateStarting
	e.events.Emit(EventStarting, nil)

	var err error

	switch Mode(e.cfg.Mode) {
	case ModeServer:
		err = e.startServer()
	case ModeClient:
		err = e.startClient()
	default:
		err = fmt.Errorf("unknown mode: %s", e.cfg.Mode)
	}

	if err != nil {
		e.state = StateError
		e.lastError = err
		e.events.Emit(EventError, err)
		return err
	}

	e.state = StateRunning
	e.events.Emit(EventStarted, nil)
	return nil
}

// Stop gracefully stops the engine
func (e *Engine) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state != StateRunning {
		return nil
	}

	e.state = StateStopping
	e.events.Emit(EventStopping, nil)

	if e.server != nil {
		e.server.Stop()
	}
	if e.client != nil {
		e.client.Stop()
	}

	e.state = StateStopped
	e.events.Emit(EventStopped, nil)
	return nil
}

// State returns the current engine state
func (e *Engine) GetState() State {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.state
}

// IsRunning returns whether the engine is running
func (e *Engine) IsRunning() bool {
	return e.GetState() == StateRunning
}

// LastError returns the last error
func (e *Engine) LastError() error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastError
}

// OnEvent registers an event handler
func (e *Engine) OnEvent(event EventType, handler EventHandler) {
	e.events.On(event, handler)
}

// GetConfig returns the current configuration (read-only)
func (e *Engine) GetConfig() *config.Config {
	return e.cfg
}

// startServer initializes and starts the server
func (e *Engine) startServer() error {
	srv, err := server.New(e.cfg, e.log)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	if err := srv.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	e.server = srv
	e.log.Info("🟢 Server started on %s (transport: %s)", e.cfg.BindAddr, e.cfg.Transport)
	return nil
}

// startClient initializes and starts the client
func (e *Engine) startClient() error {
	cli, err := client.New(e.cfg, e.log)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	if err := cli.Start(); err != nil {
		return fmt.Errorf("failed to start client: %w", err)
	}

	e.client = cli
	e.log.Info("🟢 Client connected to %s (transport: %s)", e.cfg.RemoteAddr, e.cfg.Transport)
	return nil
}
