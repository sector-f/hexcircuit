package main

import (
	"crypto/rand"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sector-f/hexgrid/generators/pcb"
)

//go:embed index.html
var indexHTML string

const maxRequestBody = 1 << 20 // 1 MiB

var (
	indexTemplate = template.Must(template.New("index").Parse(indexHTML))
	paintPattern  = regexp.MustCompile("^(#[0-9a-fA-F]{3}([0-9a-fA-F]{3})?|[A-Za-z]{1,32})$")
)

type generateRequest struct {
	Width                               float64 `json:"width"`
	Height                              float64 `json:"height"`
	BaseColor                           string  `json:"baseColor"`
	AccentColor                         string  `json:"accentColor"`
	Background                          string  `json:"background"`
	ComponentCount                      int     `json:"componentCount"`
	ClusterCount                        int     `json:"clusterCount"`
	MinComponentsPerCluster             int     `json:"minComponentsPerCluster"`
	MinPowerSourcesPerCluster            int     `json:"minPowerSourcesPerCluster"`
	MaxPowerSourcesPerCluster            int     `json:"maxPowerSourcesPerCluster"`
	ComponentRadius                     float64 `json:"componentRadius"`
	ComponentStrokeWidth                float64 `json:"componentStrokeWidth"`
	ComponentMiddleHexagonScale         float64 `json:"componentMiddleHexagonScale"`
	ComponentInnerHexagonScale          float64 `json:"componentInnerHexagonScale"`
	Connections                         int     `json:"connections"`
	InterClusterTraces                  bool    `json:"interClusterTraces"`
	MaxParallel                         int     `json:"maxParallel"`
	Pitch                               float64 `json:"pitch"`
	TraceStrokeWidth                    float64 `json:"traceStrokeWidth"`
	TraceSpacing                        float64 `json:"traceSpacing"`
	NodeRadius                          float64 `json:"nodeRadius"`
	ShowBackgroundGrid                  bool    `json:"showBackgroundGrid"`
	OmitPartialGridHexagons             bool    `json:"omitPartialGridHexagons"`
	GridRadius                          float64 `json:"gridRadius"`
	GridStrokeWidth                     float64 `json:"gridStrokeWidth"`
	TopPowerSourceCount                 int     `json:"topPowerSourceCount"`
	BottomPowerSourceCount              int     `json:"bottomPowerSourceCount"`
	PowerSourceSpacing                  string  `json:"powerSourceSpacing"`
	PowerSourceStyle                    string  `json:"powerSourceStyle"`
	ConstrainComponentsToPowerSourceSpan bool   `json:"constrainComponentsToPowerSourceSpan"`
	ConstrainLayoutToPowerSourceTrapezoid bool  `json:"constrainLayoutToPowerSourceTrapezoid"`
	ConnectorInset                      float64 `json:"connectorInset"`
	Seed                                string  `json:"seed"`
	SeedSet                             bool    `json:"seedSet"`
}

type generateResponse struct {
	SVG string `json:"svg"`
}

type pageData struct {
	Seed string
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "HTTP listen address")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", handleIndex)
	mux.HandleFunc("POST /api/generate", handleGenerate)

	server := &http.Server{
		Addr:              *addr,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("PCB layout web UI available at http://%s", *addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' blob: data:; connect-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	seed, err := randomSeed()
	if err != nil {
		http.Error(w, "could not initialize random seed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := indexTemplate.Execute(w, pageData{Seed: strconv.FormatInt(seed, 10)}); err != nil {
		log.Printf("render index template: %v", err)
	}
}

func handleGenerate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var request generateRequest
	if err := decoder.Decode(&request); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeJSONError(w, http.StatusBadRequest, "invalid request: expected a single JSON object")
		return
	}

	options, err := request.options()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	svgText, _, _, err := pcb.Generate(options)
	if err != nil {
		writeJSONError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(generateResponse{SVG: svgText}); err != nil {
		log.Printf("write generation response: %v", err)
	}
}

func (r generateRequest) options() (pcb.Options, error) {
	if !positiveFinite(r.Width) || r.Width > 100 || !positiveFinite(r.Height) || r.Height > 100 {
		return pcb.Options{}, fmt.Errorf("canvas width and height must be greater than 0 and no more than 100 inches")
	}
	if !paintPattern.MatchString(r.BaseColor) || !paintPattern.MatchString(r.AccentColor) {
		return pcb.Options{}, fmt.Errorf("base and accent colors must be hex colors or CSS color names")
	}
	if r.Background != "none" && r.Background != "transparent" && !paintPattern.MatchString(r.Background) {
		return pcb.Options{}, fmt.Errorf("background must be none, transparent, a hex color, or a CSS color name")
	}
	if r.ComponentCount < 1 || r.ComponentCount > 500 {
		return pcb.Options{}, fmt.Errorf("component count must be between 1 and 500")
	}
	if r.ClusterCount < 1 || r.ClusterCount > 50 {
		return pcb.Options{}, fmt.Errorf("cluster count must be between 1 and 50")
	}
	if r.MinComponentsPerCluster < 1 || r.MinComponentsPerCluster > 500 ||
		r.ClusterCount > r.ComponentCount/r.MinComponentsPerCluster {
		return pcb.Options{}, fmt.Errorf("component count must support the requested cluster count and minimum cluster size")
	}
	if r.Connections < 0 || r.Connections > 1000 {
		return pcb.Options{}, fmt.Errorf("extra connection count must be between 0 and 1000")
	}
	if r.MaxParallel < 1 || r.MaxParallel > 3 {
		return pcb.Options{}, fmt.Errorf("maximum parallel tracks must be between 1 and 3")
	}
	if r.MinPowerSourcesPerCluster < 0 || r.MaxPowerSourcesPerCluster < r.MinPowerSourcesPerCluster || r.MaxPowerSourcesPerCluster > 6 {
		return pcb.Options{}, fmt.Errorf("power sources per cluster must satisfy 0 <= minimum <= maximum <= 6")
	}
	if r.TopPowerSourceCount < 0 || r.TopPowerSourceCount > 100 ||
		r.BottomPowerSourceCount < 0 || r.BottomPowerSourceCount > 100 ||
		r.TopPowerSourceCount+r.BottomPowerSourceCount < r.ClusterCount*r.MinPowerSourcesPerCluster {
		return pcb.Options{}, fmt.Errorf("top and bottom contact counts must support the requested minimum power sources per cluster")
	}
	if r.ConstrainComponentsToPowerSourceSpan && r.TopPowerSourceCount+r.BottomPowerSourceCount == 0 {
		return pcb.Options{}, fmt.Errorf("the active-source span constraint requires at least one configured power-source contact")
	}
	spacing := pcb.PowerSourceSpacing(r.PowerSourceSpacing)
	if spacing != pcb.PowerSourceSpacingAdjacent && spacing != pcb.PowerSourceSpacingSeparated {
		return pcb.Options{}, fmt.Errorf("power-source spacing must be %q or %q", pcb.PowerSourceSpacingAdjacent, pcb.PowerSourceSpacingSeparated)
	}
	style := pcb.PowerSourceStyle(r.PowerSourceStyle)
	if style != pcb.PowerSourceStyleSingle && style != pcb.PowerSourceStyleDouble {
		return pcb.Options{}, fmt.Errorf("power-source style must be %q or %q", pcb.PowerSourceStyleSingle, pcb.PowerSourceStyleDouble)
	}
	if r.ConstrainLayoutToPowerSourceTrapezoid && (r.TopPowerSourceCount == 0 || r.BottomPowerSourceCount == 0) {
		return pcb.Options{}, fmt.Errorf("the trapezoid constraint requires both top and bottom power-source rows")
	}
	if !positiveFinite(r.Pitch) || !positiveFinite(r.ComponentRadius) ||
		!positiveFinite(r.ComponentStrokeWidth) || !positiveFinite(r.TraceStrokeWidth) ||
		!positiveFinite(r.TraceSpacing) || !positiveFinite(r.NodeRadius) ||
		!positiveFinite(r.GridRadius) || !positiveFinite(r.ConnectorInset) {
		return pcb.Options{}, fmt.Errorf("pitch, component geometry, trace geometry, grid radius, and connector inset must be greater than 0")
	}
	if !nonnegativeFinite(r.GridStrokeWidth) {
		return pcb.Options{}, fmt.Errorf("grid stroke width cannot be negative")
	}
	if !validScale(r.ComponentMiddleHexagonScale) || !validScale(r.ComponentInnerHexagonScale) ||
		r.ComponentInnerHexagonScale >= r.ComponentMiddleHexagonScale {
		return pcb.Options{}, fmt.Errorf("component inner scales must satisfy 0 < inner < middle < 1")
	}

	var seed int64
	if r.SeedSet {
		if strings.TrimSpace(r.Seed) == "" {
			return pcb.Options{}, fmt.Errorf("enter an integer seed or turn off the fixed-seed option")
		}
		parsedSeed, err := strconv.ParseInt(strings.TrimSpace(r.Seed), 10, 64)
		if err != nil {
			return pcb.Options{}, fmt.Errorf("seed must be a signed 64-bit integer")
		}
		seed = parsedSeed
	}

	return pcb.Options{
		Width:                                r.Width,
		Height:                               r.Height,
		BaseColor:                            r.BaseColor,
		AccentColor:                          r.AccentColor,
		Background:                           r.Background,
		ComponentCount:                       r.ComponentCount,
		ClusterCount:                         r.ClusterCount,
		MinComponentsPerCluster:              r.MinComponentsPerCluster,
		MinPowerSourcesPerCluster:             r.MinPowerSourcesPerCluster,
		MaxPowerSourcesPerCluster:             r.MaxPowerSourcesPerCluster,
		Connections:                          r.Connections,
		InterClusterTraces:                   r.InterClusterTraces,
		MaxParallel:                          r.MaxParallel,
		Pitch:                                r.Pitch,
		ComponentRadius:                      r.ComponentRadius,
		ComponentStrokeWidth:                 r.ComponentStrokeWidth,
		ComponentMiddleHexagonScale:          r.ComponentMiddleHexagonScale,
		ComponentInnerHexagonScale:           r.ComponentInnerHexagonScale,
		TraceStrokeWidth:                     r.TraceStrokeWidth,
		TraceSpacing:                         r.TraceSpacing,
		NodeRadius:                           r.NodeRadius,
		ShowBackgroundGrid:                   r.ShowBackgroundGrid,
		OmitPartialGridHexagons:              r.OmitPartialGridHexagons,
		GridRadius:                           r.GridRadius,
		GridStrokeWidth:                      r.GridStrokeWidth,
		TopPowerSourceCount:                  r.TopPowerSourceCount,
		BottomPowerSourceCount:               r.BottomPowerSourceCount,
		PowerSourceSpacing:                   spacing,
		PowerSourceStyle:                     style,
		ConstrainComponentsToPowerSourceSpan: r.ConstrainComponentsToPowerSourceSpan,
		ConstrainLayoutToPowerSourceTrapezoid: r.ConstrainLayoutToPowerSourceTrapezoid,
		ConnectorInset:                       r.ConnectorInset,
		Seed:                                 seed,
		SeedSet:                              r.SeedSet,
	}, nil
}

func validScale(value float64) bool {
	return positiveFinite(value) && value < 1
}

func positiveFinite(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func nonnegativeFinite(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func randomSeed() (int64, error) {
	var data [8]byte
	if _, err := rand.Read(data[:]); err != nil {
		return 0, err
	}
	return int64(binary.LittleEndian.Uint64(data[:])), nil
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
