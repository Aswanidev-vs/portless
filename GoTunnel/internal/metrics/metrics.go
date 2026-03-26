package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

type Registry struct {
	counters sync.Map
	gauges   sync.Map
}

func New() *Registry {
	return &Registry{}
}

func (r *Registry) IncCounter(name string) {
	ptr, _ := r.counters.LoadOrStore(name, new(uint64))
	atomic.AddUint64(ptr.(*uint64), 1)
}

func (r *Registry) AddCounter(name string, n uint64) {
	ptr, _ := r.counters.LoadOrStore(name, new(uint64))
	atomic.AddUint64(ptr.(*uint64), n)
}

func (r *Registry) SetGauge(name string, value float64) {
	ptr, _ := r.gauges.LoadOrStore(name, new(uint64))
	atomic.StoreUint64(ptr.(*uint64), uint64(value*1000))
}

func (r *Registry) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		lines := []string{}
		r.counters.Range(func(key, value any) bool {
			lines = append(lines, fmt.Sprintf("%s %d", sanitize(key.(string)), atomic.LoadUint64(value.(*uint64))))
			return true
		})
		r.gauges.Range(func(key, value any) bool {
			lines = append(lines, fmt.Sprintf("%s %.3f", sanitize(key.(string)), float64(atomic.LoadUint64(value.(*uint64)))/1000))
			return true
		})
		sort.Strings(lines)
		_, _ = w.Write([]byte(strings.Join(lines, "\n") + "\n"))
	}
}

func sanitize(in string) string {
	replacer := strings.NewReplacer("-", "_", ".", "_", " ", "_", "/", "_")
	return replacer.Replace(in)
}
