// Package control exposes a small HTTP interface for driving a running
// mdview window from the outside: press keys, move the mouse, open files,
// read the viewer's state and timings, and save a screenshot.
//
// It exists for automated checks. A developer or an AI agent cannot see the
// window or touch the keyboard, so without an interface like this the only
// way to test a change is to ask a human to look. The endpoints deliberately
// drive the same code paths the real keyboard and mouse drive, so a test
// through this interface tells you something about the real program.
//
// The server binds to 127.0.0.1 only and starts only when mdview is launched
// with -control-port, so a normal run has no listener and no overhead.
package control

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// API is the part of the viewer the control interface drives. Every method
// runs the same code the real keyboard, mouse and file watcher run.
type API interface {
	// State reports what is on screen right now.
	State() any
	// Metrics reports how long mdview's own work took, per stage.
	Metrics() any
	// Key delivers one key press. mods holds any of "ctrl", "alt", "shift".
	Key(name string, mods []string) error
	// Mouse delivers one mouse action: "move", "down", "up", "wheel",
	// "middle", or "drag" (from x,y by dx,dy).
	Mouse(action string, x, y, dx, dy, delta int) error
	// Eval runs JavaScript in the Markdown page and returns its value as
	// JSON. It fails while an image is on screen.
	Eval(js string, timeout time.Duration) (json.RawMessage, error)
	// Open displays a file, as clicking a link to it would. It returns the
	// message the user would see, empty when the file opened.
	Open(path string) (string, error)
	// Resize sets the window's outer size in physical pixels.
	Resize(w, h int) error
	// Focus brings the window to the front.
	Focus() error
	// Screenshot saves a PNG of the window.
	Screenshot(path string, clientOnly bool, mode string) error
	// Quit closes the window and ends the process.
	Quit() error
}

// PortFile is where Serve writes the port it listened on, so a test script
// can find a running mdview without being told the port.
func PortFile() string {
	return filepath.Join(os.TempDir(), "mdview-control.port")
}

// Serve starts the control server. port 0 picks a free port. It returns the
// port actually in use; the server itself runs in the background.
func Serve(port int, api API) (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return 0, err
	}
	actual := ln.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	registerRoutes(mux, api)

	if err := os.WriteFile(PortFile(), []byte(strconv.Itoa(actual)), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "control: cannot write port file: %v\n", err)
	}

	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	return actual, nil
}

func registerRoutes(mux *http.ServeMux, api API) {
	mux.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {
		reply(w, api.State(), nil)
	})

	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		reply(w, api.Metrics(), nil)
	})

	mux.HandleFunc("/key", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		name := q.Get("k")
		if name == "" {
			reply(w, nil, fmt.Errorf("missing k"))
			return
		}
		var mods []string
		if s := q.Get("mods"); s != "" {
			mods = strings.Split(s, ",")
		}
		reply(w, "ok", api.Key(name, mods))
	})

	mux.HandleFunc("/mouse", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		action := q.Get("a")
		if action == "" {
			reply(w, nil, fmt.Errorf("missing a"))
			return
		}
		reply(w, "ok", api.Mouse(action,
			intParam(q.Get("x"), 0), intParam(q.Get("y"), 0),
			intParam(q.Get("dx"), 0), intParam(q.Get("dy"), 0),
			intParam(q.Get("delta"), 120)))
	})

	mux.HandleFunc("/eval", func(w http.ResponseWriter, r *http.Request) {
		js := r.URL.Query().Get("js")
		if js == "" {
			body, _ := readAll(r)
			js = body
		}
		if strings.TrimSpace(js) == "" {
			reply(w, nil, fmt.Errorf("missing js"))
			return
		}
		timeout := time.Duration(intParam(r.URL.Query().Get("timeoutMs"), 5000)) * time.Millisecond
		val, err := api.Eval(js, timeout)
		reply(w, val, err)
	})

	mux.HandleFunc("/open", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if path == "" {
			reply(w, nil, fmt.Errorf("missing path"))
			return
		}
		msg, err := api.Open(path)
		reply(w, map[string]string{"message": msg}, err)
	})

	mux.HandleFunc("/resize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		width, height := intParam(q.Get("w"), 0), intParam(q.Get("h"), 0)
		if width <= 0 || height <= 0 {
			reply(w, nil, fmt.Errorf("w and h must be positive"))
			return
		}
		reply(w, "ok", api.Resize(width, height))
	})

	mux.HandleFunc("/focus", func(w http.ResponseWriter, r *http.Request) {
		reply(w, "ok", api.Focus())
	})

	mux.HandleFunc("/screenshot", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		path := q.Get("path")
		if path == "" {
			reply(w, nil, fmt.Errorf("missing path"))
			return
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			reply(w, nil, err)
			return
		}
		err = api.Screenshot(abs, q.Get("client") == "1", q.Get("mode"))
		reply(w, map[string]string{"path": abs}, err)
	})

	mux.HandleFunc("/quit", func(w http.ResponseWriter, r *http.Request) {
		reply(w, "ok", api.Quit())
	})
}

type response struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

func reply(w http.ResponseWriter, data any, err error) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		enc.Encode(response{OK: false, Error: err.Error()})
		return
	}
	enc.Encode(response{OK: true, Data: data})
}

func intParam(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

// maxBodyBytes caps a request body. Scripts sent to /eval are short; the cap
// only keeps a stray large POST from being read into memory.
const maxBodyBytes = 1 << 20

func readAll(r *http.Request) (string, error) {
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	return string(body), err
}
