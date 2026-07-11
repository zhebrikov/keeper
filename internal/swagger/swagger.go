// Package swagger serves OpenAPI documentation and Swagger UI.
package swagger

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
)

//go:generate make -C ../.. proto

//go:embed openapi/gophkeeper.swagger.json
var openAPI embed.FS

//go:embed ui
var ui embed.FS

// Handler returns an HTTP handler for Swagger UI and the OpenAPI spec.
func Handler() (http.Handler, error) {
	uiFS, err := fs.Sub(ui, "ui")
	if err != nil {
		return nil, fmt.Errorf("swagger ui fs: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		data, err := openAPI.ReadFile("openapi/gophkeeper.swagger.json")
		if err != nil {
			http.Error(w, "openapi spec not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	})
	mux.Handle("/", http.FileServer(http.FS(uiFS)))

	return mux, nil
}
