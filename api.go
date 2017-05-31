// Copyright 2016 Square Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package keysync

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/square/go-sq-metrics"
)

var (
	httpPost = []string{"POST"}
	httpGet  = []string{"HEAD", "GET"}
)

// APIServer holds state needed for responding to HTTP api requests
type APIServer struct {
	syncer *Syncer
	logger *logrus.Entry
}

func (a *APIServer) syncAll(w http.ResponseWriter, r *http.Request) {
	err := a.syncer.RunOnce()
	if err != nil {
		a.logger.WithError(err).Warn("Error syncing")
		http.Error(w, fmt.Sprintf("Error syncing: %v", err), http.StatusInternalServerError)
		return
	}

	// TODO: Produce output - some kind of JSON status object
}

func (a *APIServer) syncOne(w http.ResponseWriter, r *http.Request) {
	client, hasClient := mux.Vars(r)["client"]
	if !hasClient || client == "" {
		// Should be unreachable
		a.logger.Info("Invalid request: No client provided.")
		http.Error(w, "Invalid request: No client provided.", http.StatusBadRequest)
		return
	}
	logger := a.logger.WithField("client", client)
	logger.Info("Syncing one")
	a.syncer.syncMutex.Lock()
	defer a.syncer.syncMutex.Unlock()

	err := a.syncer.LoadClients()
	if err != nil {
		logger.WithError(err).Warn("Failed while loading clients")
		http.Error(w, fmt.Sprintf("Failed while loading clients: %v", err), http.StatusInternalServerError)
		return
	}

	syncerEntry, ok := a.syncer.clients[client]
	if !ok {
		logger.Info("Unknown client")
		http.Error(w, fmt.Sprintf("Unknown client %s", client), http.StatusNotFound)
		return
	}
	err = syncerEntry.Sync()
	if err != nil {
		logger.WithError(err).Warn("Error syncing")
		http.Error(w, fmt.Sprintf("Error syncing %s: %v", client, err), http.StatusInternalServerError)
		return
	}

	// TODO: Produce output - some kind of JSON status object
}

func (a *APIServer) status(w http.ResponseWriter, r *http.Request) {
	// TODO: Produce output - some kind of JSON status object
}

func (a *APIServer) health(w http.ResponseWriter, r *http.Request) {
	// TODO: only reply 200 OK if we've had some success.
	w.Write([]byte("OK"))
}

// handle wraps the HandlerFunc with logging, and registers it in the given router.
func handle(router *mux.Router, path string, methods []string, fn http.HandlerFunc, logger *logrus.Entry) {
	wrapped := func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		fn(w, r)
		logger.WithFields(logrus.Fields{
			"url":      r.URL,
			"duration": time.Since(start),
		}).Info("Request")
	}
	router.HandleFunc(path, wrapped).Methods(methods...)
}

// NewAPIServer is the constructor for an APIServer
func NewAPIServer(syncer *Syncer, port uint16, baseLogger *logrus.Entry, metrics *sqmetrics.SquareMetrics) {
	logger := baseLogger.WithField("logger", "api_server")
	apiServer := APIServer{syncer: syncer, logger: logger}
	router := mux.NewRouter()

	// Debug endpoints
	handle(router, "/debug/pprof", httpGet, pprof.Index, logger)
	handle(router, "/debug/pprof/cmdline", httpGet, pprof.Cmdline, logger)
	handle(router, "/debug/pprof/profile", httpGet, pprof.Profile, logger)
	handle(router, "/debug/pprof/symbol", httpGet, pprof.Symbol, logger)

	// Sync endpoints
	handle(router, "/sync", httpPost, apiServer.syncAll, logger)
	handle(router, "/sync/{client}", httpPost, apiServer.syncOne, logger)

	// Status and metrics endpoints
	// /_status is expected by our deploy system, and should return a minimal response.
	handle(router, "/status", httpGet, apiServer.status, logger)
	handle(router, "/_status", httpGet, apiServer.health, logger)
	handle(router, "/metrics", httpGet, metrics.ServeHTTP, logger)

	go func() {
		err := http.ListenAndServe(fmt.Sprintf("localhost:%d", port), router)
		logger.WithError(err).WithField("port", port).Error("Listen and Serve")
	}()
}
