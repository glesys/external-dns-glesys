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

package webhook

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

const (
	mediaTypeFormat        = "application/external.dns.webhook+json;"
	contentTypeHeader      = "Content-Type"
	contentTypePlaintext   = "text/plain"
	acceptHeader           = "Accept"
	varyHeader             = "Vary"
	supportedMediaVersions = "1"
	healthPath             = "/health"
	logFieldRequestPath    = "requestPath"
	logFieldRequestMethod  = "requestMethod"
	logFieldError          = "error"
)

var mediaTypeVersion1 = mediaTypeVersion("1")

type mediaType string

func mediaTypeVersion(v string) mediaType {
	return mediaType(mediaTypeFormat + "version=" + v)
}

func (m mediaType) Is(headerValue string) bool {
	return string(m) == headerValue
}

func checkAndGetMediaTypeHeaderValue(value string) (string, error) {
	for _, v := range strings.Split(supportedMediaVersions, ",") {
		if mediaTypeVersion(v).Is(value) {
			return v, nil
		}
	}
	supportedMediaTypesString := ""
	for i, v := range strings.Split(supportedMediaVersions, ",") {
		sep := ""
		if i < len(supportedMediaVersions)-1 {
			sep = ", "
		}
		supportedMediaTypesString += string(mediaTypeVersion(v)) + sep
	}
	return "", fmt.Errorf("unsupported media type version: '%s'. Supported media types are: '%s'", value, supportedMediaTypesString)
}

// Webhook for external dns provider
type Webhook struct {
	provider provider.Provider
}

// New creates a new instance of the Webhook
func New(provider provider.Provider) *Webhook {
	p := Webhook{provider: provider}
	return &p
}

func Health(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == healthPath {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (p *Webhook) contentTypeHeaderCheck(w http.ResponseWriter, r *http.Request) error {
	return p.headerCheck(true, w, r)
}

func (p *Webhook) acceptHeaderCheck(w http.ResponseWriter, r *http.Request) error {
	return p.headerCheck(false, w, r)
}

func (p *Webhook) headerCheck(isContentType bool, w http.ResponseWriter, r *http.Request) error {
	var header string
	if isContentType {
		header = r.Header.Get(contentTypeHeader)
	} else {
		header = r.Header.Get(acceptHeader)
	}
	if len(header) == 0 {
		w.Header().Set(contentTypeHeader, contentTypePlaintext)
		w.WriteHeader(http.StatusNotAcceptable)
		msg := "client must provide "
		if isContentType {
			msg += "a content type"
		} else {
			msg += "an accept header"
		}
		err := fmt.Errorf(msg)
		_, writeErr := fmt.Fprint(w, err.Error())
		if writeErr != nil {
			requestLog(r).WithField(logFieldError, writeErr).Fatalf("error writing error message to response writer")
		}
		return err
	}
	// as we support only one media type version, we can ignore the returned value
	if _, err := checkAndGetMediaTypeHeaderValue(header); err != nil {
		w.Header().Set(contentTypeHeader, contentTypePlaintext)
		w.WriteHeader(http.StatusUnsupportedMediaType)
		msg := "client must provide a valid versioned media type in the "
		if isContentType {
			msg += "content type"
		} else {
			msg += "accept header"
		}
		err := fmt.Errorf(msg+": %s", err.Error())
		_, writeErr := fmt.Fprint(w, err.Error())
		if writeErr != nil {
			requestLog(r).WithField(logFieldError, writeErr).Fatalf("error writing error message to response writer")
		}
		return err
	}
	return nil
}

// Records handles the get request for records
func (p *Webhook) Records(w http.ResponseWriter, r *http.Request) {
	if err := p.acceptHeaderCheck(w, r); err != nil {
		requestLog(r).WithField(logFieldError, err).Error("accept header check failed")
		return
	}
	requestLog(r).Debug("requesting records")
	ctx := r.Context()
	records, err := p.provider.Records(ctx)
	if err != nil {
		requestLog(r).WithField(logFieldError, err).Error("error getting records")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	requestLog(r).Debugf("returning records count: %d", len(records))
	w.Header().Set(contentTypeHeader, string(mediaTypeVersion1))
	w.Header().Set(varyHeader, contentTypeHeader)
	err = json.NewEncoder(w).Encode(records)
	if err != nil {
		requestLog(r).WithField(logFieldError, err).Error("error encoding records")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

// ApplyChanges handles the post request for record changes
func (p *Webhook) ApplyChanges(w http.ResponseWriter, r *http.Request) {
	if err := p.contentTypeHeaderCheck(w, r); err != nil {
		requestLog(r).WithField(logFieldError, err).Error("content type header check failed")
		return
	}
	var changes plan.Changes
	ctx := r.Context()
	if err := json.NewDecoder(r.Body).Decode(&changes); err != nil {
		w.Header().Set(contentTypeHeader, contentTypePlaintext)
		w.WriteHeader(http.StatusBadRequest)
		errMsg := fmt.Sprintf("error decoding changes: %s", err.Error())
		if _, writeError := fmt.Fprint(w, errMsg); writeError != nil {
			requestLog(r).WithField(logFieldError, writeError).Fatalf("error writing error message to response writer")
		}
		requestLog(r).WithField(logFieldError, err).Info(errMsg)
		return
	}
	requestLog(r).Debugf("requesting apply changes, create: %d , updateOld: %d, updateNew: %d, delete: %d",
		len(changes.Create), len(changes.UpdateOld), len(changes.UpdateNew), len(changes.Delete))
	if err := p.provider.ApplyChanges(ctx, &changes); err != nil {
		w.Header().Set(contentTypeHeader, contentTypePlaintext)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AdjustEndpoints handles the post request for adjusting endpoints
func (p *Webhook) AdjustEndpoints(w http.ResponseWriter, r *http.Request) {
	if err := p.contentTypeHeaderCheck(w, r); err != nil {
		log.Errorf("content type header check failed, request method: %s, request path: %s", r.Method, r.URL.Path)
		return
	}
	if err := p.acceptHeaderCheck(w, r); err != nil {
		log.Errorf("accept header check failed, request method: %s, request path: %s", r.Method, r.URL.Path)
		return
	}

	var pve []*endpoint.Endpoint
	if err := json.NewDecoder(r.Body).Decode(&pve); err != nil {
		w.Header().Set(contentTypeHeader, contentTypePlaintext)
		w.WriteHeader(http.StatusBadRequest)
		errMessage := fmt.Sprintf("failed to decode request body: %v", err)
		log.Infof(errMessage+" , request method: %s, request path: %s", r.Method, r.URL.Path)
		if _, writeError := fmt.Fprint(w, errMessage); writeError != nil {
			requestLog(r).WithField(logFieldError, writeError).Fatalf("error writing error message to response writer")
		}
		return
	}
	log.Debugf("requesting adjust endpoints count: %d", len(pve))
	pve, err := p.provider.AdjustEndpoints(pve)
	if err != nil {
		log.Errorf("Failed to call adjust endpoints: %v", err)
		w.Header().Set(contentTypeHeader, contentTypePlaintext)
		w.WriteHeader(http.StatusInternalServerError)
	}
	out, _ := json.Marshal(&pve)
	log.Debugf("return adjust endpoints response, resultEndpointCount: %d", len(pve))
	w.Header().Set(contentTypeHeader, string(mediaTypeVersion1))
	w.Header().Set(varyHeader, contentTypeHeader)
	if _, writeError := fmt.Fprint(w, string(out)); writeError != nil {
		requestLog(r).WithField(logFieldError, writeError).Fatalf("error writing response")
	}
}

func (p *Webhook) Negotiate(w http.ResponseWriter, r *http.Request) {
	if err := p.acceptHeaderCheck(w, r); err != nil {
		requestLog(r).WithField(logFieldError, err).Error("accept header check failed")
		return
	}
	b, err := json.Marshal(p.provider.GetDomainFilter())
	if err != nil {
		log.Errorf("failed to marshal domain filter, request method: %s, request path: %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set(contentTypeHeader, string(mediaTypeVersion1))
	if _, writeError := w.Write(b); writeError != nil {
		requestLog(r).WithField(logFieldError, writeError).Error("error writing response")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func requestLog(r *http.Request) *log.Entry {
	return log.WithFields(log.Fields{logFieldRequestMethod: r.Method, logFieldRequestPath: r.URL.Path})
}
