package api

import (
	"embed"
	"net/http"
)

//go:embed openapi.yaml
var openapiFS embed.FS

// OpenAPISchema serves the embedded OpenAPI document so the contract cannot
// drift from the binary that serves it.
func OpenAPISchema() http.HandlerFunc {
	b, err := openapiFS.ReadFile("openapi.yaml")
	if err != nil {
		// embed guarantees presence; failure is a compile-time invariant.
		return func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "openapi document missing", http.StatusInternalServerError)
		}
	}
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		w.Header().Set("Cache-Control", "public, max-age=60")
		_, _ = w.Write(b)
	}
}
