package engine

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"
)

func (e *Engine) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /", e.handleIndex)
	mux.HandleFunc("GET /monitor", e.handleMonitor)
	mux.HandleFunc("GET /api/monitor/jobs", e.handleMonitorJobs)

	mux.HandleFunc("GET /healthz", e.handleHealthz)
	mux.HandleFunc("GET /readyz", e.handleReadyz)

	return mux
}

type healthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}

func (e *Engine) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC(),
	})
}

func (e *Engine) handleReadyz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status:    "ready",
		Timestamp: time.Now().UTC(),
	})
}

func (e *Engine) handleIndex(w http.ResponseWriter, r *http.Request) {
	serveEmbeddedHTML(w, "web/index.html")
}

func (e *Engine) handleMonitor(w http.ResponseWriter, r *http.Request) {
	serveEmbeddedHTML(w, "web/monitor.html")
}

func (e *Engine) handleMonitorJobs(w http.ResponseWriter, r *http.Request) {
	if e.scheduler == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"jobs": []any{},
			"meta": map[string]any{
				"enabled": false,
				"message": "scheduler disabled: set CONFIG_FILE to enable backup jobs",
			},
		})
		return
	}

	jobs := e.scheduler.MonitorSnapshot()

	counts := map[string]int{"running": 0, "failed": 0, "idle": 0}
	for _, job := range jobs {
		counts[job.State]++
	}

	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].State != jobs[j].State {
			return jobs[i].State < jobs[j].State
		}
		return jobs[i].Name < jobs[j].Name
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"jobs": jobs,
		"meta": map[string]any{
			"enabled": true,
			"counts":  counts,
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
