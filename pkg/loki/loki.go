package loki

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"

	"github.com/probe-lab/thunderdome/pkg/prom"
)

const (
	tailPath = "/loki/api/v1/tail"
)

// LokiTailer reads a stream of nginx logs from Loki
type LokiTailer struct {
	cfg                     LokiConfig
	ch                      chan LogLine
	shutdown                chan struct{} // semaphore to indicate that shutdown has been called
	requestsDroppedCounter  prometheus.Counter
	requestsIncomingCounter prometheus.Counter
	errorCounter            prometheus.Counter
	connectedGauge          prometheus.Gauge

	mu   sync.Mutex // guards following fields
	conn *websocket.Conn
}

type LokiConfig struct {
	AppName   string
	URI       string // URI of the loki server, e.g. https://logs-prod-us-central1.grafana.net
	Username  string // For grafana cloud this is a numeric user id
	Password  string // For grafana cloud this is the API token
	Query     string // the query to use to obtain logs
	QueryTags string
	OrgID     string
	TLSConfig config.TLSConfig
}

type LogLine struct {
	Server       string            `json:"server"`
	Time         time.Time         `json:"time"`
	Method       string            `json:"method"`
	URI          string            `json:"uri"`
	Status       int               `json:"status"`
	Headers      map[string]string `json:"headers"`
	RemoteAddr   string            `json:"addr"`
	UserAgent    string            `json:"agent"`
	Referer      string            `json:"referer"`
	RespBodySize int               `json:"resp_body_size"`
	RespTime     float32           `json:"resp_time"`
}

func NewLokiTailer(cfg *LokiConfig) (*LokiTailer, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config must not be nil")
	}
	l := &LokiTailer{
		cfg:      *cfg,
		shutdown: make(chan struct{}),
		ch:       make(chan LogLine, 100*60*30),
	}

	commonLabels := map[string]string{}
	var err error

	l.requestsIncomingCounter, err = prom.NewPrometheusCounter(
		cfg.AppName,
		"loki_requests_incoming_total",
		"The total number of requests read from loki.",
		commonLabels,
	)
	if err != nil {
		return nil, fmt.Errorf("new counter: %w", err)
	}

	l.requestsDroppedCounter, err = prom.NewPrometheusCounter(
		cfg.AppName,
		"loki_requests_dropped_total",
		"The total number of requests that could not be sent to the publisher.",
		commonLabels,
	)
	if err != nil {
		return nil, fmt.Errorf("new counter: %w", err)
	}

	l.errorCounter, err = prom.NewPrometheusCounter(
		cfg.AppName,
		"loki_error_total",
		"The total number of errors encountered when reading from loki.",
		commonLabels,
	)
	if err != nil {
		return nil, fmt.Errorf("new counter: %w", err)
	}

	l.connectedGauge, err = prom.NewPrometheusGauge(
		cfg.AppName,
		"loki_connected",
		"Indicates whether the tailer is connected to loki.",
		commonLabels,
	)
	if err != nil {
		return nil, fmt.Errorf("new gauge: %w", err)
	}

	return l, nil
}

func (l *LokiTailer) Chan() <-chan LogLine {
	return l.ch
}

func (l *LokiTailer) Run(ctx context.Context) error {
	if err := l.connect(); err != nil {
		return err
	}
	defer l.connectedGauge.Set(0)

	defer close(l.ch)

	go func() {
		<-ctx.Done()
		l.Shutdown(context.Background())
	}()

	for {
		tr, err := l.readTailResponse()
		if errors.Is(ctx.Err(), context.Canceled) {
			return ctx.Err()
		} else if err != nil {
			log.Printf("failed to read tail from loki: %v", err)
			if err := l.connect(); err != nil {
				l.errorCounter.Add(1)
				log.Printf("failed to connect to loki: %v", err)
				time.Sleep(30 * time.Second)
			}
			continue
		}
		for _, stream := range tr.Streams {
			for _, entry := range stream.Values {
				l.requestsIncomingCounter.Add(1)

				var line LogLine
				err := json.Unmarshal([]byte(entry.Line()), &line)
				if err != nil {
					l.errorCounter.Add(1)
					log.Printf("failed to parse loki json: %v", err)
					continue
				}

				select {
				case <-l.shutdown:
					// shutdown was called
					return nil
				case <-ctx.Done():
					l.Shutdown(context.Background())
					return ctx.Err()
				case l.ch <- line:
				}

			}
		}

	}
}

func (l *LokiTailer) connect() error {
	// Clean up if we were previously connected
	l.mu.Lock()
	if l.conn != nil {
		l.conn.Close()
		l.connectedGauge.Set(0)
	}
	l.conn = nil
	l.mu.Unlock()

	const limit = 100
	const delayFor = time.Duration(0)

	params := url.Values{}
	params.Set("query", l.cfg.Query)
	if delayFor != 0 {
		params.Set("delay_for", strconv.FormatInt(int64(delayFor.Seconds()), 10))
	}
	params.Set("limit", strconv.FormatInt(int64(limit), 10))
	params.Set("start", strconv.FormatInt(time.Now().Add(-60*time.Minute).UnixNano(), 10))

	us, err := buildURL(l.cfg.URI, tailPath, params.Encode())
	if err != nil {
		return fmt.Errorf("build url: %w", err)
	}

	tlsConfig, err := config.NewTLSConfig(&l.cfg.TLSConfig)
	if err != nil {
		return fmt.Errorf("new tls config: %w", err)
	}

	if strings.HasPrefix(us, "http") {
		us = strings.Replace(us, "http", "ws", 1)
	}

	h, err := l.getHTTPRequestHeader()
	if err != nil {
		return fmt.Errorf("http headers: %w", err)
	}

	ws := websocket.Dialer{
		TLSClientConfig: tlsConfig,
	}

	conn, resp, err := ws.Dial(us, h)
	if err != nil {
		if resp == nil {
			return fmt.Errorf("missing http response: %w", err)
		}
		buf, _ := io.ReadAll(resp.Body) // nolint
		return fmt.Errorf("error response from server: %s (%v)", string(buf), err)
	}

	l.mu.Lock()
	l.conn = conn
	l.mu.Unlock()
	l.connectedGauge.Set(1)
	log.Printf("connected to loki using %s", l.cfg.URI)
	return nil
}

func (l *LokiTailer) Shutdown(ctx context.Context) error {
	close(l.shutdown)
	defer l.connectedGauge.Set(0)

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.conn == nil {
		return nil
	}

	if err := l.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")); err != nil {
		l.conn = nil
		return fmt.Errorf("write close message: %w", err)
	}
	l.conn.Close()
	l.conn = nil
	return nil
}

func (l *LokiTailer) readTailResponse() (*TailResponse, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.conn == nil {
		return nil, fmt.Errorf("attempted to read while disconnected")
	}

	tr := new(TailResponse)
	_, data, err := l.conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("read message: %w", err)
	}

	if err := json.Unmarshal(data, tr); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}

	return tr, nil
}

func (l *LokiTailer) getHTTPRequestHeader() (http.Header, error) {
	h := make(http.Header)

	if l.cfg.Username != "" && l.cfg.Password != "" {
		h.Set(
			"Authorization",
			"Basic "+base64.StdEncoding.EncodeToString([]byte(l.cfg.Username+":"+l.cfg.Password)),
		)
	}

	h.Set("User-Agent", fmt.Sprintf("%s/0.1", l.cfg.AppName))

	if l.cfg.OrgID != "" {
		h.Set("X-Scope-OrgID", l.cfg.OrgID)
	}

	if l.cfg.QueryTags != "" {
		h.Set("X-Query-Tags", l.cfg.QueryTags)
	}

	return h, nil
}

type TailResponse struct {
	Streams []Stream `json:"streams,omitempty"`
}

type Stream struct {
	Labels map[string]string `json:"stream"`
	Values []ValueTuple      `json:"values"`
}

type ValueTuple [2]string

func (v ValueTuple) Line() string {
	return v[1]
}

func buildURL(u, p, q string) (string, error) {
	url, err := url.Parse(u)
	if err != nil {
		return "", err
	}
	url.Path = path.Join(url.Path, p)
	url.RawQuery = q
	return url.String(), nil
}
