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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/pprof"
	"strings"
	"time"

	"github.com/square/keysync/backup"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	sqmetrics "github.com/square/go-sq-metrics"
)

var (
	httpPost = []string{"POST"}
	httpGet  = []string{"HEAD", "GET"}
)

const (
	pollIntervalFailureThresholdMultiplier = 10
)

// APIServer holds state needed for responding to HTTP api requests
type APIServer struct {
	backup backup.Backup
	syncer *Syncer
	logger *logrus.Entry
}

// StatusResponse from API endpoints
type StatusResponse struct {
	Ok      bool     `json:"ok"`
	Message string   `json:"message,omitempty"`
	Updated *Updated `json:"updated,omitempty"`
}

func writeSuccess(w http.ResponseWriter, updated *Updated) {
	resp := &StatusResponse{Ok: true, Updated: updated}
	out, _ := json.MarshalIndent(resp, "", "  ")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out)
	_, _ = w.Write([]byte("\n"))
}

func writeError(w http.ResponseWriter, status int, err error) {
	resp := &StatusResponse{Ok: false, Message: err.Error()}
	out, _ := json.MarshalIndent(resp, "", "")
	w.WriteHeader(status)
	_, _ = w.Write(out)
	_, _ = w.Write([]byte("\n"))
}

func (a *APIServer) syncAll(w http.ResponseWriter, r *http.Request) {
	a.logger.Info("Syncing all from API")
	updated, errs := a.syncer.RunOnce()
	if len(errs) != 0 {
		err := fmt.Errorf("errors: %v", errs)
		a.logger.WithError(err).Warn("error syncing")
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeSuccess(w, &updated)
}

func (a *APIServer) syncOne(w http.ResponseWriter, r *http.Request) {
	client, hasClient := mux.Vars(r)["client"]
	if !hasClient || client == "" {
		// Should be unreachable
		a.logger.Info("Invalid request: No client provided.")
		writeError(w, http.StatusBadRequest, errors.New("invalid request: no client provided"))
		return
	}

	// Sanitize the user-controlled client string for logging to prevent log message forgeries.
	sanitizedClient := strings.ReplaceAll(client, "\n", "")
	sanitizedClient = strings.ReplaceAll(sanitizedClient, "\r", "")

	logger := a.logger.WithField("client", sanitizedClient)
	logger.Info("Syncing one")
	a.syncer.syncMutex.Lock()
	defer a.syncer.syncMutex.Unlock()

	pendingCleanup, err := a.syncer.LoadClients()
	if err != nil {
		logger.WithError(err).Warn("Failed while loading clients")
		writeError(w, http.StatusInternalServerError, fmt.Errorf("failed while loading clients: %s", err))
		return
	}
	// We do this in a defer because we want it to run regardless of which of the
	// below cases we end up in.
	defer pendingCleanup.cleanup(a.logger)

	var updated Updated
	if syncerEntry, ok := a.syncer.clients[client]; ok {
		updated, err = syncerEntry.Sync()
		if err != nil {
			logger.WithError(err).Warnf("Error syncing %s", sanitizedClient)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("error syncing %s: %s", sanitizedClient, err))
			return
		}
	} else if _, pending := pendingCleanup.Outputs[client]; !pending {
		// If it's not a current client, or one pending cleanup, return an error
		logger.Infof("Unknown client: %s", sanitizedClient)
		writeError(w, http.StatusNotFound, fmt.Errorf("unknown client: %s", sanitizedClient))
		return
	}

	logger.WithFields(logrus.Fields{
		"Added":   updated.Added,
		"Changed": updated.Changed,
		"Deleted": updated.Deleted,
	}).Info("API requested sync complete")

	writeSuccess(w, &updated)
}

func (a *APIServer) runBackup(w http.ResponseWriter, r *http.Request) {
	if a.backup == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("Backups not configured"))
		return
	}

	if err := a.backup.Backup(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
	} else {
		writeSuccess(w, nil)
	}
}

func (a *APIServer) status(w http.ResponseWriter, r *http.Request) {
	lastSuccess, ok := a.syncer.timeSinceLastSuccess()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, errors.New("initial sync has not yet completed"))
		return
	}

	failureThreshold := a.syncer.pollInterval * pollIntervalFailureThresholdMultiplier
	if lastSuccess > failureThreshold {
		err := a.syncer.mostRecentError()
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("haven't synced in over %d seconds (most recent err: %s)", int64(lastSuccess/time.Second), err))
		return
	}

	writeSuccess(w, nil)
}

// handle wraps the HandlerFunc with logging, and registers it in the given router.
func handle(router *mux.Router, path string, methods []string, fn http.HandlerFunc, logger *logrus.Entry) {
	wrapped := func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		fn(w, r)
		logger.WithFields(logrus.Fields{
			"url":      r.URL.String(),
			"duration": time.Since(start),
		}).Info("Request")
	}
	router.HandleFunc(path, wrapped).Methods(methods...)
}

// NewAPIServer is the constructor for an APIServer
func NewAPIServer(syncer *Syncer, backup backup.Backup, port uint16, baseLogger *logrus.Entry, metrics *sqmetrics.SquareMetrics) {
	logger := baseLogger.WithField("logger", "api_server")
	apiServer := APIServer{syncer: syncer, logger: logger, backup: backup}
	router := mux.NewRouter()

	// Debug endpoints
	handle(router, "/debug/pprof", httpGet, pprof.Index, logger)
	handle(router, "/debug/pprof/cmdline", httpGet, pprof.Cmdline, logger)
	handle(router, "/debug/pprof/profile", httpGet, pprof.Profile, logger)
	handle(router, "/debug/pprof/symbol", httpGet, pprof.Symbol, logger)

	// Sync endpoints
	handle(router, "/sync", httpPost, apiServer.syncAll, logger)
	handle(router, "/sync/{client}", httpPost, apiServer.syncOne, logger)

	// Create backup
	handle(router, "/backup", httpPost, apiServer.runBackup, logger)

	// Status and metrics endpoints
	router.HandleFunc("/status", apiServer.status).Methods(httpGet...)
	handle(router, "/metrics", httpGet, metrics.ServeHTTP, logger)

	go func() {
		err := http.ListenAndServe(fmt.Sprintf("localhost:%d", port), router)
		logger.WithError(err).WithField("port", port).Error("Listen and Serve")
	}()
}
