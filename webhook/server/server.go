/*
Copyright 2023 GleSYS AB

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package server

import (
	"context"
	"errors"
	"fmt"
	"github.com/glesys/external-dns-glesys/webhook"
	"github.com/glesys/external-dns-glesys/webhook/configuration"
	"github.com/go-chi/chi/v5"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
)

// Init server initialization function
// The server will respond to the following endpoints:
// - / (GET): initialization, negotiates headers and returns the domain filter
// - /records (GET): returns the current records
// - /records (POST): applies the changes
// - /adjustendpoints (POST): executes the AdjustEndpoints method
func Init(config configuration.Config, p *webhook.Webhook) *http.Server {
	r := chi.NewRouter()
	r.Use(webhook.Health)
	r.Get("/", p.Negotiate)
	r.Get("/records", p.Records)
	r.Post("/records", p.ApplyChanges)
	r.Post("/adjustendpoints", p.AdjustEndpoints)

	srv := createHTTPServer(fmt.Sprintf("%s:%d", config.ServerHost, config.ServerPort), r, config.ServerReadTimeout, config.ServerWriteTimeout)
	go func() {
		log.Infof("starting server on addr: '%s' ", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("can't serve on addr: '%s', error: %v", srv.Addr, err)
		}
	}()
	return srv
}

func createHTTPServer(addr string, hand http.Handler, readTimeout, writeTimeout time.Duration) *http.Server {
	return &http.Server{
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		Addr:         addr,
		Handler:      hand,
	}
}

// ShutdownGracefully gracefully shutdown the http server
func ShutdownGracefully(srv *http.Server) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	sig := <-sigCh
	log.Infof("shutting down server due to received signal: %v", sig)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := srv.Shutdown(ctx); err != nil {
		log.Errorf("error shutting down server: %v", err)
	}
	cancel()
}
