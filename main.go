/**
* Copyright 2018 Comcast Cable Communications Management, LLC
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You may obtain a copy of the License at
* http://www.apache.org/licenses/LICENSE-2.0
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
 */

package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

const (
	applicationName    = "trickster"
	applicationVersion = "0.1.7"

	// Log fields
	lfEvent    = "event"
	lfDetail   = "detail"
	lfCacheKey = "cacheKey"

	// Prometheus API method names
	mnQueryRange = "query_range"
	mnQuery      = "query"
	mnLabels     = "label/__name__/values"
	mnHealth     = "health"

	// Prometheus URL endpoints
	prometheusAPIv1Path = "/api/v1/"
)

func main() {
	t := &TricksterHandler{}
	t.ResponseChannels = make(map[string]chan *ClientRequestContext)

	t.Config = NewConfig()
	if err := loadConfiguration(t.Config, os.Args[1:]); err != nil {
		// using fmt.Println because logger can't be instantiated without the config loaded
		// to know the log path, and the config load just failed, so we just abort.
		fmt.Println("Could not load trickster configuration: ", err.Error())
		os.Exit(1)
	}

	if t.Config.Main.InstanceID > 0 {
		t.Logger = newLogger(t.Config.Logging, fmt.Sprint(t.Config.Main.InstanceID))
	} else {
		t.Logger = newLogger(t.Config.Logging, "")
	}

	level.Info(t.Logger).Log("event", "application startup", "version", applicationVersion)

	if t.Config.Profiler.Enabled {
		go exposeProfilerEndpoint(t.Config, t.Logger)
	}

	t.Metrics = NewApplicationMetrics()
	t.Metrics.ListenAndServe(t.Config, t.Logger)

	t.Cacher = getCache(t)
	if err := t.Cacher.Connect(); err != nil {
		level.Error(t.Logger).Log("event", "Unable to connect to Cache", "detail", err.Error())
		os.Exit(1)
	}
	defer t.Cacher.Close()

	traceFlush, err := newTracer(t.Config.Tracing, t.Logger)
	if err != nil {
		level.Error(t.Logger).Log("event", "Unable to initialize tracing", "detail", err)
		os.Exit(1)
	}
	defer traceFlush()

	router := mux.NewRouter()

	// Health Check Paths
	router.HandleFunc("/ping", t.pingHandler).Methods("GET")
	router.HandleFunc("/{originMoniker}/"+mnHealth, t.promHealthCheckHandler).Methods("GET")
	router.HandleFunc("/"+mnHealth, t.promHealthCheckHandler).Methods("GET")

	// Path-based  multi-origin support - no support for full proxy of the prometheus UI, only querying
	router.HandleFunc("/{originMoniker}"+prometheusAPIv1Path+mnQueryRange, t.promQueryRangeHandler).Methods("GET", "POST")
	router.HandleFunc("/{originMoniker}"+prometheusAPIv1Path+mnQuery, t.promQueryHandler).Methods("GET", "POST")
	router.PathPrefix("/{originMoniker}" + prometheusAPIv1Path).HandlerFunc(t.promFullProxyHandler).Methods("GET")

	router.HandleFunc(prometheusAPIv1Path+mnQueryRange, t.promQueryRangeHandler).Methods("GET", "POST")
	router.HandleFunc(prometheusAPIv1Path+mnQuery, t.promQueryHandler).Methods("GET", "POST")
	router.PathPrefix(prometheusAPIv1Path).HandlerFunc(t.promFullProxyHandler).Methods("GET")

	// Catch All for Single-Origin proxy
	router.PathPrefix("/").HandlerFunc(t.promFullProxyHandler).Methods("GET")

	level.Info(t.Logger).Log("event", "proxy http endpoint starting", "address", t.Config.ProxyServer.ListenAddress, "port", t.Config.ProxyServer.ListenPort)

	// Start the Server
	err = http.ListenAndServe(fmt.Sprintf("%s:%d", t.Config.ProxyServer.ListenAddress, t.Config.ProxyServer.ListenPort), handlers.CompressHandler(router))
	level.Error(t.Logger).Log("event", "exiting", "err", err)
}

func exposeProfilerEndpoint(c *Config, l log.Logger) {
	level.Info(l).Log("event", "profiler http endpoint starting", "port", c.Profiler.ListenPort)
	err := http.ListenAndServe(fmt.Sprintf(":%d", c.Profiler.ListenPort), nil)
	if err != nil {
		level.Error(l).Log("event", "error starting profiler http server", "detail", err.Error())
	}
}
