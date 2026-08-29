package viewer

import (
	"sync"
	"time"
)

// Stage names recorded in Metrics. Keep them short: they are JSON keys read
// by the control interface.
const (
	stageReadFile  = "readFile"  // reading the Markdown file from disk
	stageRender    = "render"    // Markdown -> HTML page string
	stageWriteHTML = "writeHTML" // writing the page to the temp file
	stageNavigate  = "navigate"  // handing the temp file to WebView2
	stageDecode    = "decode"    // reading and decoding an image file
	stagePack      = "pack"      // compositing an image into a blittable buffer
	stagePaint     = "paint"     // one repaint of the image control
)

// stageTiming accumulates wall-clock time for one stage across the life of
// the process. Times are milliseconds.
type stageTiming struct {
	Count  int     `json:"count"`
	LastMs float64 `json:"lastMs"`
	MaxMs  float64 `json:"maxMs"`
	SumMs  float64 `json:"sumMs"`
}

func (s *stageTiming) add(d time.Duration) {
	ms := float64(d.Microseconds()) / 1000
	s.Count++
	s.LastMs = ms
	s.SumMs += ms
	if ms > s.MaxMs {
		s.MaxMs = ms
	}
}

// metricsStore holds the timings mdview measures for its own work: how long
// Go spent turning a file into something on screen. It does not measure what
// WebView2 spends laying out the page — read that from the page itself with
// the control interface's eval endpoint.
//
// Safe for use from any goroutine.
type metricsStore struct {
	mu     sync.Mutex
	stages map[string]*stageTiming
	sizes  map[string]int
}

func newMetricsStore() *metricsStore {
	return &metricsStore{
		stages: map[string]*stageTiming{},
		sizes:  map[string]int{},
	}
}

// track returns a function that records the time elapsed until it is called.
// Use it as `defer m.track(stageRender)()`.
func (m *metricsStore) track(stage string) func() {
	start := time.Now()
	return func() { m.add(stage, time.Since(start)) }
}

func (m *metricsStore) add(stage string, d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.stages[stage]
	if s == nil {
		s = &stageTiming{}
		m.stages[stage] = s
	}
	s.add(d)
}

// setSize records a size (bytes, pixels) that explains the timings beside it.
func (m *metricsStore) setSize(name string, v int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sizes[name] = v
}

// MetricsSnapshot is a copy of the collected timings, safe to serialize.
type MetricsSnapshot struct {
	Stages map[string]stageTiming `json:"stages"`
	Sizes  map[string]int         `json:"sizes"`
}

func (m *metricsStore) snapshot() MetricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := MetricsSnapshot{
		Stages: make(map[string]stageTiming, len(m.stages)),
		Sizes:  make(map[string]int, len(m.sizes)),
	}
	for k, v := range m.stages {
		out.Stages[k] = *v
	}
	for k, v := range m.sizes {
		out.Sizes[k] = v
	}
	return out
}
