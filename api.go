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

package main

import (
	"fmt"
	"log"
	"net/http"

	"net/http/pprof"

	"github.com/getsentry/raven-go"
	"github.com/gorilla/mux"
)

// APIServer holds state needed for responding to HTTP api requests
type APIServer struct {
	syncer *Syncer
}

func (a *APIServer) syncAll(w http.ResponseWriter, r *http.Request) {
	err := a.syncer.RunOnce()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error syncing: %v", err), http.StatusInternalServerError)
		raven.CaptureError(err, nil)
	}

	// TODO: Produce output - some kind of JSON status object
}

func (a *APIServer) syncOne(w http.ResponseWriter, r *http.Request) {
	client, hasClient := mux.Vars(r)["client"]
	if !hasClient || client == "" {
		http.Error(w, "Invalid request: Please provide a client", http.StatusBadRequest)
		return
	}
	a.syncer.syncMutex.Lock()
	defer a.syncer.syncMutex.Unlock()

	err := a.syncer.LoadClients()
	if err != nil {
		http.Error(w, fmt.Sprintf("Loading clients: %v", err), http.StatusInternalServerError)
		raven.CaptureError(err, nil)
	}

	syncerEntry, ok := a.syncer.clients[client]
	if !ok {
		http.Error(w, fmt.Sprintf("Unknown client %s", client), http.StatusNotFound)
	}
	err = syncerEntry.Sync()
	if err != nil {
		http.Error(w, fmt.Sprintf("Syncing: %v", err), http.StatusInternalServerError)
		raven.CaptureError(err, nil)
	}

	// TODO: Produce output - some kind of JSON status object
}

func (a *APIServer) status(w http.ResponseWriter, r *http.Request) {
	// TODO: Produce output - some kind of JSON status object
}

// NewAPIServer is the constructor for an APIServer
func NewAPIServer(syncer *Syncer, port uint16) {
	apiServer := APIServer{syncer: syncer}
	router := mux.NewRouter()

	router.HandleFunc("/debug/pprof/", pprof.Index)
	router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	router.HandleFunc("/debug/pprof/profile", pprof.Profile)
	router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)

	router.HandleFunc("/sync", apiServer.syncAll)
	router.HandleFunc("/sync/{client}", apiServer.syncOne)
	router.HandleFunc("/status", apiServer.status)
	// /_status is expected by our deploy system, and should return a minimal response.
	router.HandleFunc("/_status", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	go func() {
		// Handles the routes registered above, as well as pprof
		log.Println(http.ListenAndServe(fmt.Sprintf("localhost:%d", port), nil))
	}()
}
