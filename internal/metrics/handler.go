package metrics

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Handler serves /metrics. A scrape may narrow the output:
//
//	/metrics?host=*.example.com&host=api.example.org   only these hosts
//	/metrics?exclude_host=staging.*                    leave these out
//	/metrics?collect[]=certificates&collect[]=nginx    only these collectors
//
// which Prometheus can set per job with `params:` in scrape_configs.
func (e *Exporter) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		reg, err := e.Registry(r.Context(), Filter{Include: q["host"], Exclude: q["exclude_host"]}, q["collect[]"])
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		promhttp.HandlerFor(reg, promhttp.HandlerOpts{ErrorHandling: promhttp.ContinueOnError}).ServeHTTP(w, r)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, "<html><body><h1>GoNix exporter</h1><p><a href=\"/metrics\">Metrics</a> (instance %s)</p></body></html>\n", e.opt.Instance)
	})
	return mux
}

// Serve listens on addr until ctx is cancelled.
func (e *Exporter) Serve(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: e.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
