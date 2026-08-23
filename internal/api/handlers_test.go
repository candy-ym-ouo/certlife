package api

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeOptionalAcceptsChunkedBody(t *testing.T) {
	req := httptest.NewRequest("POST", "/", io.NopCloser(strings.NewReader(`{"force":true}`)))
	req.ContentLength = -1
	var body struct {
		Force bool `json:"force"`
	}
	if err := decodeOptional(req, &body); err != nil {
		t.Fatal(err)
	}
	if !body.Force {
		t.Fatal("force was ignored for chunked request")
	}
}

func TestDecodeRejectsOversizedBody(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"value":"`+strings.Repeat("x", 1<<20)+`"}`))
	var body map[string]any
	if err := decode(req, &body); err == nil {
		t.Fatal("oversized request body was accepted")
	}
}

func TestDecodeRejectsMultipleJSONValues(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{} {}`))
	var body map[string]any
	if err := decode(req, &body); err == nil {
		t.Fatal("multiple JSON values were accepted")
	}
}
