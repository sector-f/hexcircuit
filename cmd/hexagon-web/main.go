package main

import (
	"crypto/rand"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	hexsvg "github.com/sector-f/hexcircuit/circuit"
)

//go:embed index.html
var indexHTML string

const maxRequestBody = 1 << 20 // 1 MiB

var (
	indexTemplate = template.Must(template.New("index").Parse(indexHTML))
	paintPattern  = regexp.MustCompile(`^(#[0-9a-fA-F]{3}([0-9a-fA-F]{3})?|[A-Za-z]{1,32})$`)
)

type generateRequest struct {
	Width                     float64 `json:"width"`
	Height                    float64 `json:"height"`
	Color                     string  `json:"color"`
	Background                string  `json:"background"`
	Orientation               string  `json:"orientation"`
	ComponentCount            int     `json:"componentCount"`
	ComponentRadius           float64 `json:"componentRadius"`
	RoutingPitch              float64 `json:"routingPitch"`
	AutoRoutingPitch          bool    `json:"autoRoutingPitch"`
	GridRadius                float64 `json:"gridRadius"`
	GridGap                   float64 `json:"gridGap"`
	GridWidth                 float64 `json:"gridWidth"`
	OmitPartialGridHexagons   bool    `json:"omitPartialGridHexagons"`
	Connections               int     `json:"connections"`
	MaxParallel               int     `json:"maxParallel"`
	TrackCountWeights         string  `json:"trackCountWeights"`
	ParallelSpacing           float64 `json:"parallelSpacing"`
	StrokeWidth               float64 `json:"strokeWidth"`
	TraceWidth                float64 `json:"traceWidth"`
	NodeRadius                float64 `json:"nodeRadius"`
	TraceClearance            float64 `json:"traceClearance"`
	DetailProb                float64 `json:"detailProb"`
	ShowLabels                bool    `json:"showLabels"`
	LabelFont                 string  `json:"labelFont"`
	LabelFontSize             float64 `json:"labelFontSize"`
	LabelProbability          float64 `json:"labelProbability"`
	FontColor                 string  `json:"fontColor"`
	FontColorSpecified        bool    `json:"fontColorSpecified"`
	LabelDistance             float64 `json:"labelDistance"`
	LabelInsideComponents     bool    `json:"labelInsideComponents"`
	UseDeterministicRouteCost bool    `json:"useDeterministicRouteCost"`
	FallbackEdgeOrder         string  `json:"fallbackEdgeOrder"`
	MaxRouteSteps             int     `json:"maxRouteSteps"`
	RouteAttempts             int     `json:"routeAttempts"`
	Seed                      string  `json:"seed"`
	SeedSet                   bool    `json:"seedSet"`
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

	log.Printf("Circuit motif web UI available at http://%s", *addr)
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

	options, err := request.options()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	svgText, _, _, err := hexsvg.Generate(options)
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

func (r generateRequest) options() (hexsvg.Options, error) {
	if !positiveFinite(r.Width) || r.Width > 100 || !positiveFinite(r.Height) || r.Height > 100 {
		return hexsvg.Options{}, fmt.Errorf("canvas width and height must be greater than 0 and no more than 100 inches")
	}
	if r.Orientation != "pointy" && r.Orientation != "flat" {
		return hexsvg.Options{}, fmt.Errorf("orientation must be pointy or flat")
	}
	if !paintPattern.MatchString(r.Color) {
		return hexsvg.Options{}, fmt.Errorf("color must be a hex color or a CSS color name")
	}
	if r.Background != "none" && r.Background != "transparent" && !paintPattern.MatchString(r.Background) {
		return hexsvg.Options{}, fmt.Errorf("background must be none, transparent, a hex color, or a CSS color name")
	}
	if r.ComponentCount < 0 || r.ComponentCount > 500 {
		return hexsvg.Options{}, fmt.Errorf("component count must be between 0 and 500")
	}
	if !positiveFinite(r.ComponentRadius) {
		return hexsvg.Options{}, fmt.Errorf("component radius must be greater than 0")
	}
	if !positiveFinite(r.RoutingPitch) {
		return hexsvg.Options{}, fmt.Errorf("routing pitch must be greater than 0")
	}
	if !positiveFinite(r.GridRadius) || !nonnegativeFinite(r.GridGap) || !nonnegativeFinite(r.GridWidth) {
		return hexsvg.Options{}, fmt.Errorf("grid radius must be greater than 0; grid gap and grid width cannot be negative")
	}
	if r.Connections < 0 || r.Connections > 1000 {
		return hexsvg.Options{}, fmt.Errorf("connections must be between 0 and 1000")
	}
	if r.MaxParallel < 1 || r.MaxParallel > 12 {
		return hexsvg.Options{}, fmt.Errorf("max parallel must be between 1 and 12")
	}
	if !nonnegativeFinite(r.ParallelSpacing) || !positiveFinite(r.StrokeWidth) || !positiveFinite(r.TraceWidth) || !positiveFinite(r.NodeRadius) || !nonnegativeFinite(r.TraceClearance) {
		return hexsvg.Options{}, fmt.Errorf("parallel spacing and trace clearance cannot be negative; stroke width, trace width, and node radius must be greater than 0")
	}
	if !nonnegativeFinite(r.DetailProb) || r.DetailProb > 1 {
		return hexsvg.Options{}, fmt.Errorf("detail probability must be between 0 and 1")
	}
	if !positiveFinite(r.LabelFontSize) {
		return hexsvg.Options{}, fmt.Errorf("label font size must be greater than 0")
	}
	if !nonnegativeFinite(r.LabelProbability) || r.LabelProbability > 1 {
		return hexsvg.Options{}, fmt.Errorf("label probability must be between 0 and 1")
	}
	if !nonnegativeFinite(r.LabelDistance) {
		return hexsvg.Options{}, fmt.Errorf("label distance cannot be negative")
	}
	if r.ShowLabels && strings.TrimSpace(r.LabelFont) == "" {
		return hexsvg.Options{}, fmt.Errorf("font family cannot be empty when labels are enabled")
	}
	if r.FontColorSpecified && !paintPattern.MatchString(r.FontColor) {
		return hexsvg.Options{}, fmt.Errorf("font color must be a hex color or a CSS color name")
	}
	if r.MaxRouteSteps < 1 || r.MaxRouteSteps > 1000 {
		return hexsvg.Options{}, fmt.Errorf("max route steps must be between 1 and 1000")
	}
	if r.RouteAttempts < 1 || r.RouteAttempts > 1000 {
		return hexsvg.Options{}, fmt.Errorf("route attempts must be between 1 and 1000")
	}
	fallbackEdgeOrder := hexsvg.FallbackEdgeOrder(r.FallbackEdgeOrder)
	if fallbackEdgeOrder == "" {
		fallbackEdgeOrder = hexsvg.FallbackEdgeOrderRandomized
	}
	if fallbackEdgeOrder != hexsvg.FallbackEdgeOrderRandomized && fallbackEdgeOrder != hexsvg.FallbackEdgeOrderRanked {
		return hexsvg.Options{}, fmt.Errorf("fallback edge order must be %q or %q", hexsvg.FallbackEdgeOrderRandomized, hexsvg.FallbackEdgeOrderRanked)
	}

	weights, err := parseWeights(r.TrackCountWeights, r.MaxParallel)
	if err != nil {
		return hexsvg.Options{}, err
	}

	var seed int64
	if r.SeedSet {
		if strings.TrimSpace(r.Seed) == "" {
			return hexsvg.Options{}, fmt.Errorf("enter an integer seed or turn off the fixed-seed option")
		}
		seed, err = strconv.ParseInt(strings.TrimSpace(r.Seed), 10, 64)
		if err != nil {
			return hexsvg.Options{}, fmt.Errorf("seed must be a signed 64-bit integer")
		}
	}

	return hexsvg.Options{
		Width:                     r.Width,
		Height:                    r.Height,
		Color:                     r.Color,
		Background:                r.Background,
		Orientation:               r.Orientation,
		ComponentCount:            r.ComponentCount,
		ComponentRadius:           r.ComponentRadius,
		RoutingPitch:              r.RoutingPitch,
		AutoRoutingPitch:          r.AutoRoutingPitch,
		GridRadius:                r.GridRadius,
		GridGap:                   r.GridGap,
		GridWidth:                 r.GridWidth,
		OmitPartialGridHexagons:   r.OmitPartialGridHexagons,
		Connections:               r.Connections,
		MaxParallel:               r.MaxParallel,
		TrackCountWeights:         weights,
		ParallelSpacing:           r.ParallelSpacing,
		StrokeWidth:               r.StrokeWidth,
		TraceWidth:                r.TraceWidth,
		NodeRadius:                r.NodeRadius,
		TraceClearance:            r.TraceClearance,
		DetailProb:                r.DetailProb,
		ShowLabels:                r.ShowLabels,
		LabelFont:                 r.LabelFont,
		LabelFontSize:             r.LabelFontSize,
		LabelProbability:          r.LabelProbability,
		FontColor:                 r.FontColor,
		FontColorSpecified:        r.FontColorSpecified,
		LabelDistance:             r.LabelDistance,
		LabelInsideComponents:     r.LabelInsideComponents,
		UseDeterministicRouteCost: r.UseDeterministicRouteCost,
		FallbackEdgeOrder:         fallbackEdgeOrder,
		MaxRouteSteps:             r.MaxRouteSteps,
		RouteAttempts:             r.RouteAttempts,
		Seed:                      seed,
		SeedSet:                   r.SeedSet,
	}, nil
}

func parseWeights(value string, maxParallel int) ([]float64, error) {
	parts := strings.Split(value, ",")
	if len(parts) != maxParallel {
		return nil, fmt.Errorf("track-count weights must contain exactly %d comma-separated values", maxParallel)
	}

	weights := make([]float64, len(parts))
	sum := 0.0
	for i, part := range parts {
		weight, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil || !nonnegativeFinite(weight) {
			return nil, fmt.Errorf("track-count weights must be non-negative numbers")
		}
		weights[i] = weight
		sum += weight
	}
	if math.Abs(sum-100) > 0.01 {
		return nil, fmt.Errorf("track-count weights must add up to 100 (currently %.6g)", sum)
	}
	return weights, nil
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
