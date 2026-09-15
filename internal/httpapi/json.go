package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
)

type envelope map[string]any

func writeJSON(w http.ResponseWriter, status int, data any, headers http.Header) error {
	encoded, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal JSON response: %w", err)
	}
	encoded = append(encoded, '\n')

	for key, values := range headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(encoded); err != nil {
		return fmt.Errorf("write JSON response: %w", err)
	}
	return nil
}

func readJSON(w http.ResponseWriter, r *http.Request, destination any, maxBytes int64) error {
	contentType := r.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "application/json" {
		return errors.New("Content-Type must be application/json")
	}
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(destination); err != nil {
		var syntaxError *json.SyntaxError
		var typeError *json.UnmarshalTypeError
		var maxBytesError *http.MaxBytesError

		switch {
		case errors.As(err, &syntaxError):
			return fmt.Errorf("body contains malformed JSON at byte %d", syntaxError.Offset)
		case errors.Is(err, io.ErrUnexpectedEOF):
			return errors.New("body contains malformed JSON")
		case errors.As(err, &typeError):
			if typeError.Field != "" {
				return fmt.Errorf("body contains an invalid value for field %q", typeError.Field)
			}
			return fmt.Errorf("body contains an invalid JSON value at byte %d", typeError.Offset)
		case errors.Is(err, io.EOF):
			return errors.New("body must not be empty")
		case strings.HasPrefix(err.Error(), "json: unknown field "):
			return fmt.Errorf("body contains unknown field %s", strings.TrimPrefix(err.Error(), "json: unknown field "))
		case errors.As(err, &maxBytesError):
			return fmt.Errorf("body must not exceed %d bytes", maxBytesError.Limit)
		default:
			return fmt.Errorf("decode JSON body: %w", err)
		}
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("body must contain a single JSON value")
	}
	return nil
}
