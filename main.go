package main

import (
	"flag"
	"fmt"
    "github.com/aamcrae/config"
    "github.com/davecheney/gpio"
    "github.com/davecheney/gpio/rpi"
    "io"
	"log"
	"net/http"
    "os"
    "path"
    "time"
)

var port = flag.Int("port", 8080, "Web server port number")
var configFile = flag.String("config", "/etc/sprinkler", "Configuration file")
var verbose = flag.Bool("v", false, "Log more information")
var quiet = flag.Bool("q", false, "Do not log anything")
var mock = flag.Bool("mock", true, "Use mock hardware")
var static = flag.String("static", "files", "Directory for static files")

var gpioMap = map[string]int{
    "GPIO17": rpi.GPIO17,
    "GPIO21": rpi.GPIO21,
    "GPIO22": rpi.GPIO22,
    "GPIO23": rpi.GPIO23,
    "GPIO24": rpi.GPIO24,
    "GPIO25": rpi.GPIO25,
    "GPIO27": rpi.GPIO27,
}

type Log  struct {
    when time.Time
    event string
}

const (
    ON = "On"
    OFF = "Off"
)

// A Line is a single controllable irrigation line, along with
// a preset schedule.
type Line struct {
    index int
    description string
    pinName string
    state string
    pin *gpio.Pin
}

// Map that keeps track of GPIO pins in use.
var gpioUsed map[int]struct{} = map[int]struct{}{}

// A System is a set of Lines.
type System struct {
    Lines []*Line
    start time.Time
    duration time.Duration
    gap time.Duration
}

func init() {
	flag.Parse()
}

func main() {
    system, err := NewSystem(*configFile)
     if err != nil {
        log.Fatalf("%s: %v", *configFile, err)
    }
    // If we crash, make sure everything gets turned off.
    defer system.Shutdown()
    staticServer := http.FileServer(http.Dir(*static))
	http.Handle("/static/", http.StripPrefix("/static", staticServer))
	http.Handle("/favicon.ico", staticServer)
	http.Handle("/smarthome", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
            system.homeHandler(w, r)
        }))
	http.Handle("/api", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
            system.apiHandler(w, r)
        }))
	http.Handle("/index.html", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
            system.pageHandler(w, r)
        }))
    url := fmt.Sprintf(":%d", *port)
    if *verbose {
        log.Printf("Starting server on %s", url)
    }
	s := &http.Server{Addr: url}
	log.Fatal(s.ListenAndServe())
}

func NewSystem(confFile string) (* System, error) {
    if *verbose {
        log.Printf("Configuring from %s", confFile)
    }
    conf, err := config.ParseFile(confFile)
    if err != nil {
        return nil, err
    }
    s := &System{Lines:[]*Line{}}
    for i := 1; true; i++ {
        k := fmt.Sprintf("line%d", i)
        e, ok := conf.Get(k)
        if !ok {
            break
        }
        if len(e.Tokens) != 2 {
            s.Shutdown()
            return nil, fmt.Errorf("%s: Bad Line config at %d", e.Lineno)
        }
        if *verbose {
            log.Printf("New line: %s on %s", e.Tokens[0], e.Tokens[1])
        }
        line, err := NewLine(i, e.Tokens[0], e.Tokens[1])
        if err != nil {
            s.Shutdown()
            return nil, err
        }
        s.Lines = append(s.Lines, line)
    }
    if e, ok := conf.Get("start"); !ok {
        s.start = parseTime("11:00PM")
    } else {
        s.start = parseTime(e.Tokens[0])
    }
    if e, ok := conf.Get("duration"); !ok {
        s.duration = parseDuration("20m")
    } else {
        s.duration = parseDuration(e.Tokens[0])
    }
    if e, ok := conf.Get("gap"); !ok {
        s.gap = parseDuration("2m")
    } else {
        s.gap = parseDuration(e.Tokens[0])
    }
    if *verbose {
        log.Printf("Start: %s, duration %s, gap %s", s.start.String(), s.duration.String(), s.gap.String())
    }
    return s, nil
}

func (s *System) pageHandler(w http.ResponseWriter, r *http.Request) {
    if *verbose {
        log.Printf("URL request: %v", r.URL)
    }
	w.Header().Set("Content-Type", "text/html")
    file(w, "header.html")
    for _, l := range s.Lines {
        fmt.Fprintf(w, "Line %s is %s<br>", l.description, l.state)
    }
    fmt.Fprintf(w, "Start time :  %s<br>", s.start.Format("3:04PM"))
    fmt.Fprintf(w, "Duration   :  %s<br>", s.duration.String())
    fmt.Fprintf(w, "Gap        :  %s<br>", s.gap.String())
    file(w, "trailer.html")
}

func file(w http.ResponseWriter, file string) {
    if *verbose {
        log.Printf("Sending %s", file)
    }
    f, err := os.Open(path.Join(*static, file))
    if err != nil {
        log.Printf("%s: %v", file, err)
    } else {
        defer f.Close()
        _, err = io.Copy(w, f)
        if err != nil {
            log.Printf("%s: %v", file, err)
        }
    }
}

func (s *System) Shutdown() {
    for _, l := range s.Lines {
        l.Off()
    }
}

func parseTime(str string) time.Time {
    t, err := time.Parse("3:04PM", str)
    if err != nil {
        log.Fatalf("Illegal time: %s", str)
    }
    return t
}

func parseDuration(str string) time.Duration {
    d, err := time.ParseDuration(str)
    if err != nil {
        log.Fatalf("Illegal duration: %s", str)
    }
    return d
}

func NewLine(index int, description string, pinName string) (* Line, error) {
    pin, ok := gpioMap[pinName]
    if !ok {
        return nil, fmt.Errorf("Unknown pin: %s", pinName)
    }
    if _, ok := gpioUsed[pin]; ok {
        return nil, fmt.Errorf("Pin %s already in use", pinName)
    }
    l := &Line{index, description, pinName, "off", nil}
    if !*mock {
        p, err := rpi.OpenPin(pin, gpio.ModeOutput)
        if err != nil {
            return nil, fmt.Errorf("Pin %s: %v", pinName)
        }
        l.pin = &p
    }
    gpioUsed[pin] = struct{}{}
    return l, nil
}

func (l *Line) On() {
    l.setState(ON)
}

func (l *Line) Off() {
    l.setState(OFF)
}

func (l *Line) setState(state string) {
    if l.state == state {
        if *verbose {
            log.Printf("Line: %d is already %s\n", l.description, state)
        }
        return
    }
    if state == "off" {
        // Set pin off
    } else if state == "on" {
        // Set pin on
    } else {
        log.Fatalf("Illegal state: %s", state)
    }
    l.state = state
    if *verbose {
        log.Printf("Line %s: turning %s\n", l.description, state)
    }
}
