package core

import "sync"

// EventType represents the type of engine event
type EventType string

const (
	EventStarting     EventType = "starting"
	EventStarted      EventType = "started"
	EventStopping     EventType = "stopping"
	EventStopped      EventType = "stopped"
	EventError        EventType = "error"
	EventConnected    EventType = "connected"
	EventDisconnected EventType = "disconnected"
	EventReconnecting EventType = "reconnecting"
	EventPathChanged  EventType = "path_changed"
	EventProbeBlocked EventType = "probe_blocked"
	EventTraffic      EventType = "traffic"
)

// EventHandler is a callback function for events
type EventHandler func(data interface{})

// EventBus manages event subscriptions and dispatching
type EventBus struct {
	handlers map[EventType][]EventHandler
	mu       sync.RWMutex
}

// NewEventBus creates a new event bus
func NewEventBus() *EventBus {
	return &EventBus{
		handlers: make(map[EventType][]EventHandler),
	}
}

// On registers a handler for an event type
func (eb *EventBus) On(event EventType, handler EventHandler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.handlers[event] = append(eb.handlers[event], handler)
}

// Emit dispatches an event to all registered handlers
func (eb *EventBus) Emit(event EventType, data interface{}) {
	eb.mu.RLock()
	handlers := eb.handlers[event]
	eb.mu.RUnlock()

	for _, h := range handlers {
		go h(data) // Non-blocking dispatch
	}
}

// Off removes all handlers for an event type
func (eb *EventBus) Off(event EventType) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	delete(eb.handlers, event)
}

// TrafficEvent holds traffic statistics for the traffic event
type TrafficEvent struct {
	BytesUp   int64
	BytesDown int64
	UserID    string
}

// ConnectionEvent holds connection info
type ConnectionEvent struct {
	RemoteAddr string
	Transport  string
	UserID     string
}
