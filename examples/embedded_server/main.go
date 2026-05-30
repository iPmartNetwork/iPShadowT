// Example: Embedding iPShadowT engine in another application
package main

import (
	"fmt"
	"net/http"

	"github.com/iPmart/iPShadowT/core"
)

func main() {
	// Create tunnel engine
	engine, err := core.New(
		core.WithMode(core.ModeServer),
		core.WithTransport(core.TransportWSMux),
		core.WithBindAddr("0.0.0.0:443"),
		core.WithPassword("embedded-secret"),
		core.WithKernelTuning(true),
	)
	if err != nil {
		panic(err)
	}

	// Start tunnel in background
	if err := engine.Start(); err != nil {
		panic(err)
	}
	defer engine.Stop()

	// Run your own HTTP server alongside
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("My app is running, tunnel is active!"))
	})

	http.HandleFunc("/tunnel/status", func(w http.ResponseWriter, r *http.Request) {
		if engine.IsRunning() {
			w.Write([]byte(`{"status":"running"}`))
		} else {
			w.Write([]byte(`{"status":"stopped"}`))
		}
	})

	fmt.Println("App running on :8080, tunnel on :443")
	http.ListenAndServe(":8080", nil)
}
