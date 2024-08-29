package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Get("/", helloWorld)
	http.ListenAndServe(":3000", r)
}

func helloWorld(responseWriter http.ResponseWriter, r *http.Request) {
	responseWriter.Write([]byte("Hello World!11213"))
}
