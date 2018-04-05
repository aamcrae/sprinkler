package main

import (
    "encoding/json"
	"log"
	"net/http"
)

type request struct {
    Action string   `json:"action"`
    Line int        `json:"line"`       // 0 is all lines.
    Runs int        `json:"runs"`
}

type lineStatus struct {
    Line int        `json:"line"`
    Name string     `json:"name"`
    State string    `json:"state"`
    Duration int    `json:"duration"`
    LastRun string  `json:"lastrun"`
    NextRun string  `json:"nextrun"`
}

func (s *System) apiHandler(w http.ResponseWriter, r *http.Request) {
    if *verbose {
        log.Printf("API URL request: %s %v", r.Method, r.URL)
    }
    switch r.Method {
    case "GET":
        s.apiGet(w, r)
    case "POST":
        s.apiPost(w, r)
    default:
        w.WriteHeader(http.StatusBadRequest)
    }
}

func (s *System) apiGet(w http.ResponseWriter, r *http.Request) {
    m, err := json.Marshal(s.lineState(0))
    if err != nil {
        w.WriteHeader(http.StatusBadRequest)
        log.Printf("Marshal: %v", err)
        return
    }
	w.Header().Set("Content-Type", "application/json")
    w.Write(m)
}

func (s *System) lineState(index int) interface{} {
    if index > 0 {
        return s.Lines[index].lineState()
    }
    resp := []lineStatus{}
    for _, l := range s.Lines {
        resp = append(resp, l.lineState())
    }
    return resp
}

func (l *Line) lineState() lineStatus {
    return lineStatus{l.index, l.description, l.state, 20, "an hour ago", "soon"}
}

func (s *System) apiPost(w http.ResponseWriter, r *http.Request) {
    var req request
    err := json.NewDecoder(r.Body).Decode(&req)
    if err != nil {
        w.WriteHeader(http.StatusBadRequest)
        log.Printf("POST unmarshal: %v", err)
        return
    }
    if req.Line < 0 || req.Line > len(s.Lines) {
        w.WriteHeader(http.StatusBadRequest)
        log.Printf("Bad line selected (%d)", req.Line)
        return
    }
    var l *Line
    if req.Line > 0 {
        l = s.Lines[req.Line - 1]
    }
    switch req.Action {
    case "start":
        log.Printf("Turning line %d (%s) ON", req.Line, desc(l))
    case "stop":
        log.Printf("Turning line %d (%s) OFF", req.Line, desc(l))
    case "skip":
        log.Printf("Cancelling next run of line %d (%s)", req.Line, desc(l))
    default:
        w.WriteHeader(http.StatusBadRequest)
        log.Printf("Bad action (%s) selected on line %d", req.Action, req.Line)
        return
    }
    m, err := json.Marshal(s.lineState(req.Line))
    if err != nil {
        w.WriteHeader(http.StatusBadRequest)
        log.Printf("Marshal: %v", err)
        return
    }
	w.Header().Set("Content-Type", "application/json")
    w.Write(m)
}

func desc(l *Line) string {
    if l == nil {
        return "All lines"
    }
    return l.description
}
