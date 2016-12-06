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

// Expose pprof under /debug/pprof on the API port
import _ "net/http/pprof"
import (
	"fmt"
	"log"
	"net/http"
)

type ApiServer struct {
	syncer *Syncer
}

func (a *ApiServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.syncer.RunNow()
}

func NewApiServer(syncer *Syncer, port uint16) {
	apiServer := ApiServer{syncer: syncer}
	http.Handle("/sync", &apiServer)
	http.HandleFunc("/_status", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	go func() {
		// Handles the routes registered above, as well as pprof
		log.Println(http.ListenAndServe(fmt.Sprintf("localhost:%d", port), nil))
	}()
}
