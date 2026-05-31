package plugin

import (
	"fmt"
	"net"
	"sync"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// Plugin defines the interface that all plugins must implement
type Plugin interface {
	// Name returns the plugin name
	Name() string
	// Version returns the plugin version
	Version() string
	// Init initializes the plugin with configuration
	Init(config map[string]interface{}) error
	// Start starts the plugin
	Start() error
	// Stop stops the plugin
	Stop() error
}

// TransportPlugin extends Plugin for custom transport implementations
type TransportPlugin interface {
	Plugin
	// Dial creates a new connection (client-side)
	Dial(addr string) (net.Conn, error)
	// Listen starts accepting connections (server-side)
	Listen(addr string) (net.Listener, error)
}

// AuthPlugin extends Plugin for custom authentication
type AuthPlugin interface {
	Plugin
	// Authenticate checks if a connection is authorized
	Authenticate(conn net.Conn, data []byte) (userID string, ok bool, err error)
}

// FilterPlugin extends Plugin for traffic filtering/modification
type FilterPlugin interface {
	Plugin
	// OnConnect is called when a new connection is established
	OnConnect(conn net.Conn) (net.Conn, error)
	// OnData is called for each data chunk (can modify data)
	OnData(data []byte, direction Direction) ([]byte, error)
}

// Direction indicates data flow direction
type Direction int

const (
	DirectionUpstream   Direction = iota // Client → Server
	DirectionDownstream                  // Server → Client
)

// Hook defines lifecycle hooks for plugins
type Hook string

const (
	HookPreConnect    Hook = "pre_connect"
	HookPostConnect   Hook = "post_connect"
	HookPreDisconnect Hook = "pre_disconnect"
	HookOnData        Hook = "on_data"
	HookOnError       Hook = "on_error"
)

// HookHandler is a function called when a hook fires
type HookHandler func(ctx *HookContext) error

// HookContext provides context to hook handlers
type HookContext struct {
	Hook       Hook
	Connection net.Conn
	Data       []byte
	UserID     string
	Error      error
	Metadata   map[string]interface{}
}

// Registry manages plugin registration and lifecycle
type Registry struct {
	plugins    map[string]Plugin
	transports map[string]TransportPlugin
	auth       []AuthPlugin
	filters    []FilterPlugin
	hooks      map[Hook][]HookHandler
	mu         sync.RWMutex
	log        *logger.Logger
}

// NewRegistry creates a new plugin registry
func NewRegistry(log *logger.Logger) *Registry {
	return &Registry{
		plugins:    make(map[string]Plugin),
		transports: make(map[string]TransportPlugin),
		auth:       make([]AuthPlugin, 0),
		filters:    make([]FilterPlugin, 0),
		hooks:      make(map[Hook][]HookHandler),
		log:        log,
	}
}

// Register registers a plugin
func (r *Registry) Register(p Plugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := p.Name()
	if _, exists := r.plugins[name]; exists {
		return fmt.Errorf("plugin %q already registered", name)
	}

	r.plugins[name] = p

	// Register by type
	if tp, ok := p.(TransportPlugin); ok {
		r.transports[name] = tp
	}
	if ap, ok := p.(AuthPlugin); ok {
		r.auth = append(r.auth, ap)
	}
	if fp, ok := p.(FilterPlugin); ok {
		r.filters = append(r.filters, fp)
	}

	r.log.Info("Plugin registered: %s v%s", name, p.Version())
	return nil
}

// Unregister removes a plugin
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, exists := r.plugins[name]
	if !exists {
		return fmt.Errorf("plugin %q not found", name)
	}

	// Stop the plugin
	p.Stop()

	delete(r.plugins, name)
	delete(r.transports, name)

	r.log.Info("Plugin unregistered: %s", name)
	return nil
}

// GetTransport returns a registered transport plugin
func (r *Registry) GetTransport(name string) (TransportPlugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tp, ok := r.transports[name]
	return tp, ok
}

// GetPlugin returns a registered plugin by name
func (r *Registry) GetPlugin(name string) (Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[name]
	return p, ok
}

// ListPlugins returns all registered plugins
func (r *Registry) ListPlugins() []PluginInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]PluginInfo, 0, len(r.plugins))
	for _, p := range r.plugins {
		info := PluginInfo{
			Name:    p.Name(),
			Version: p.Version(),
		}
		if _, ok := p.(TransportPlugin); ok {
			info.Type = "transport"
		} else if _, ok := p.(AuthPlugin); ok {
			info.Type = "auth"
		} else if _, ok := p.(FilterPlugin); ok {
			info.Type = "filter"
		} else {
			info.Type = "generic"
		}
		infos = append(infos, info)
	}
	return infos
}

// PluginInfo holds plugin metadata
type PluginInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Type    string `json:"type"`
}

// OnHook registers a hook handler
func (r *Registry) OnHook(hook Hook, handler HookHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hooks[hook] = append(r.hooks[hook], handler)
}

// FireHook fires a hook and calls all registered handlers
func (r *Registry) FireHook(ctx *HookContext) error {
	r.mu.RLock()
	handlers := r.hooks[ctx.Hook]
	r.mu.RUnlock()

	for _, h := range handlers {
		if err := h(ctx); err != nil {
			return err
		}
	}
	return nil
}

// RunFilters applies all filter plugins to a connection
func (r *Registry) RunFilters(conn net.Conn) (net.Conn, error) {
	r.mu.RLock()
	filters := r.filters
	r.mu.RUnlock()

	var err error
	for _, f := range filters {
		conn, err = f.OnConnect(conn)
		if err != nil {
			return nil, fmt.Errorf("filter %s failed: %w", f.Name(), err)
		}
	}
	return conn, nil
}

// Authenticate runs all auth plugins
func (r *Registry) Authenticate(conn net.Conn, data []byte) (string, bool, error) {
	r.mu.RLock()
	auths := r.auth
	r.mu.RUnlock()

	for _, a := range auths {
		userID, ok, err := a.Authenticate(conn, data)
		if err != nil {
			return "", false, err
		}
		if ok {
			return userID, true, nil
		}
	}

	// No auth plugin accepted - allow by default if no auth plugins registered
	if len(auths) == 0 {
		return "", true, nil
	}

	return "", false, nil
}

// StartAll starts all registered plugins
func (r *Registry) StartAll() error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, p := range r.plugins {
		if err := p.Start(); err != nil {
			return fmt.Errorf("failed to start plugin %s: %w", name, err)
		}
		r.log.Info("Plugin started: %s", name)
	}
	return nil
}

// StopAll stops all registered plugins
func (r *Registry) StopAll() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, p := range r.plugins {
		if err := p.Stop(); err != nil {
			r.log.Error("Failed to stop plugin %s: %v", name, err)
		}
	}
}
