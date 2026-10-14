package handler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
)

// The handler tests check every /api response they get against
// api/openapi.yaml, so the spec the frontend's types come from can't drift
// from what the server sends.

var spec = sync.OnceValues(func() (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		return nil, err
	}
	return doc, doc.Validate(context.Background())
})

// checkContract validates one exchange. Redirects (the browser-facing auth
// routes) have no body to check.
func checkContract(t *testing.T, serverURL string, req *http.Request, res *reply) {
	t.Helper()
	if !strings.HasPrefix(req.URL.Path, "/api/") || res.StatusCode == http.StatusFound {
		return
	}
	doc, err := spec()
	if err != nil {
		t.Fatalf("openapi.yaml: %v", err)
	}
	router, err := gorillamux.NewRouter(withServer(doc, serverURL+"/api"))
	if err != nil {
		t.Fatal(err)
	}
	route, params, err := router.FindRoute(req)
	if err != nil {
		if res.StatusCode == http.StatusNotFound && errors.Is(err, routers.ErrPathNotFound) {
			return // unknown paths are supposed to 404
		}
		t.Fatalf("%s %s: not in openapi.yaml: %v", req.Method, req.URL.Path, err)
	}
	in := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request: req, PathParams: params, Route: route,
			Options: &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc},
		},
		Status: res.StatusCode,
		Header: res.Header,
		Body:   io.NopCloser(bytes.NewReader(res.Body)),
		Options: &openapi3filter.Options{
			IncludeResponseStatus: true,
		},
	}
	if err := openapi3filter.ValidateResponse(context.Background(), in); err != nil {
		t.Errorf("%s %s %d: response doesn't match openapi.yaml: %v", req.Method, req.URL.Path, res.StatusCode, err)
	}
}

func withServer(doc *openapi3.T, url string) *openapi3.T {
	c := *doc
	c.Servers = openapi3.Servers{{URL: url}}
	return &c
}
