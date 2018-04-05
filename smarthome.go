package main

import (
    "log"
	"net/http"
)

func (s *System) homeHandler(w http.ResponseWriter, r *http.Request) {
    if *verbose {
        log.Printf("smarthome URL request: %s %v", r.Method, r.URL)
    }
}
