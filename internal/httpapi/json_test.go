package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type jsonInput struct {
	Name string `json:"name"`
}

func TestReadJSON(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		maxBytes    int64
		wantName    string
		wantError   string
	}{
		{name: "valid", contentType: "application/json; charset=utf-8", body: `{"name":"The Place"}`, wantName: "The Place"},
		{name: "missing content type", body: `{}`, wantError: "Content-Type"},
		{name: "wrong content type", contentType: "text/plain", body: `{}`, wantError: "Content-Type"},
		{name: "empty", contentType: "application/json", body: ``, wantError: "must not be empty"},
		{name: "malformed", contentType: "application/json", body: `{"name":`, wantError: "malformed JSON"},
		{name: "wrong type", contentType: "application/json", body: `{"name":4}`, wantError: "invalid value"},
		{name: "unknown field", contentType: "application/json", body: `{"unknown":true}`, wantError: "unknown field"},
		{name: "multiple values", contentType: "application/json", body: `{} {}`, wantError: "single JSON value"},
		{name: "too large", contentType: "application/json", body: `{"name":"long"}`, maxBytes: 4, wantError: "must not exceed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(test.body))
			if test.contentType != "" {
				request.Header.Set("Content-Type", test.contentType)
			}
			response := httptest.NewRecorder()
			var input jsonInput

			err := readJSON(response, request, &input, test.maxBytes)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("readJSON() error = %v, want %q", err, test.wantError)
				}
				return
			}
			if err != nil || input.Name != test.wantName {
				t.Fatalf("readJSON() input=%+v error=%v", input, err)
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	response := httptest.NewRecorder()
	headers := make(http.Header)
	headers.Set("Location", "/api/v1/items/1")

	if err := writeJSON(response, http.StatusCreated, envelope{"data": envelope{"name": "The Place"}}, headers); err != nil {
		t.Fatalf("writeJSON() error = %v", err)
	}
	if response.Code != http.StatusCreated || response.Header().Get("Content-Type") != "application/json; charset=utf-8" || response.Header().Get("Location") == "" {
		t.Fatalf("unexpected response: code=%d headers=%v", response.Code, response.Header())
	}
	if got := response.Body.String(); got != "{\"data\":{\"name\":\"The Place\"}}\n" {
		t.Fatalf("body = %q", got)
	}
}

func TestWriteJSONRejectsUnsupportedValueBeforeWritingHeader(t *testing.T) {
	response := httptest.NewRecorder()
	err := writeJSON(response, http.StatusOK, envelope{"data": make(chan int)}, nil)
	if err == nil || response.Code != http.StatusOK || response.Body.Len() != 0 {
		t.Fatalf("writeJSON() error=%v code=%d body=%q", err, response.Code, response.Body.String())
	}
}

func BenchmarkReadJSON(b *testing.B) {
	body := `{"name":"The Place"}`
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		var input jsonInput
		if err := readJSON(httptest.NewRecorder(), request, &input, 1024); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWriteJSON(b *testing.B) {
	payload := envelope{"data": envelope{"name": "The Place"}}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := writeJSON(httptest.NewRecorder(), http.StatusOK, payload, nil); err != nil {
			b.Fatal(err)
		}
	}
}
