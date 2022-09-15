package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/gorilla/websocket"
	"github.com/grafana/loki/pkg/logcli/client"
	"github.com/grafana/loki/pkg/logcli/query"
	"github.com/grafana/loki/pkg/loghttp"
	"github.com/grafana/loki/pkg/util/unmarshal"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
)

type Request struct {
	Method    string            `json:"method"`
	URI       string            `json:"uri"`
	Body      []byte            `json:"body,omitempty"`
	Header    map[string]string `json:"header"`
	Timestamp time.Time         `json:"ts"` // time the request was created
}

// A RequestSource is a provider of a stream of requests that can be sent to workers.
type RequestSource interface {
	// Start starts the request source, makiing requests available on Chan.
	Start() error

	// Stop stops sending requests to the channel.
	Stop()

	// Chan is a channel that can be used to read requests produced by the source.
	// This channel will be closed when the stream ends.
	Chan() <-chan Request

	// Err returns any error that was encountered while processing the stream.
	Err() error

	Name() string
}

type RequestSourceMetrics struct {
	requestsDropped  prometheus.Counter
	requestsFiltered prometheus.Counter
	requestsIncoming prometheus.Counter
	errors           prometheus.Counter
	connected        prometheus.Gauge
}

func NewRequestSourceMetrics(labels map[string]string) (*RequestSourceMetrics, error) {
	s := &RequestSourceMetrics{}

	var err error
	s.requestsDropped, err = newPrometheusCounter(
		"source_requests_dropped_total",
		"The total number of requests dropped by the request source due to targets falling behind.",
		labels,
	)
	if err != nil {
		return nil, fmt.Errorf("new counter: %w", err)
	}

	s.requestsFiltered, err = newPrometheusCounter(
		"source_requests_filtered_total",
		"The total number of requests ignored by the request source due to filter rules.",
		labels,
	)
	if err != nil {
		return nil, fmt.Errorf("new counter: %w", err)
	}

	s.requestsIncoming, err = newPrometheusCounter(
		"source_requests_incoming_total",
		"The total number of requests read from the request source.",
		labels,
	)
	if err != nil {
		return nil, fmt.Errorf("new counter: %w", err)
	}

	s.errors, err = newPrometheusCounter(
		"source_error_total",
		"The total number of errors encountered when reading from request source.",
		labels,
	)
	if err != nil {
		return nil, fmt.Errorf("new counter: %w", err)
	}

	s.connected, err = newPrometheusGauge(
		"source_connected",
		"Indicates whether the request source is connected to its provider of requests.",
		labels,
	)
	if err != nil {
		return nil, fmt.Errorf("new gauge: %w", err)
	}

	return s, nil
}

// StdinRequestSource is a request source that reads a stream of JSON requests
// from stdin.
type StdinRequestSource struct {
	ch      chan Request
	done    chan struct{}
	filter  RequestFilter
	metrics *RequestSourceMetrics

	mu  sync.Mutex // guards following fields
	err error
}

var _ RequestSource = (*StdinRequestSource)(nil)

func NewStdinRequestSource(filter RequestFilter, metrics *RequestSourceMetrics) *StdinRequestSource {
	return &StdinRequestSource{
		done:    make(chan struct{}),
		ch:      make(chan Request),
		filter:  filter,
		metrics: metrics,
	}
}

func (s *StdinRequestSource) Name() string {
	return "stdin"
}

func (s *StdinRequestSource) Chan() <-chan Request {
	return s.ch
}

func (s *StdinRequestSource) Start() error {
	go func() {
		s.metrics.connected.Set(1)
		defer s.metrics.connected.Set(0)
		defer close(s.ch)

		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			s.metrics.requestsIncoming.Add(1)
			data := scanner.Bytes()
			var req Request
			if err := json.Unmarshal(data, &req); err != nil {
				s.metrics.errors.Add(1)
				log.Printf("failed to unmarshal request: %v", err)
				continue
			}

			if s.filter != nil && !s.filter(&req) {
				s.metrics.requestsFiltered.Add(1)
				continue
			}

			select {
			case <-s.done:
				return
			case s.ch <- req:
			}
		}
	}()

	return nil
}

func (s *StdinRequestSource) Stop() {
	close(s.done)
}

func (s *StdinRequestSource) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// RandomRequestSource is a request source that provides a random request
// from a list of requests.
type RandomRequestSource struct {
	name    string
	reqs    []*Request
	rng     *rand.Rand
	ch      chan Request
	done    chan struct{}
	filter  RequestFilter
	metrics *RequestSourceMetrics
}

func NewRandomRequestSource(filter RequestFilter, metrics *RequestSourceMetrics, reqs []*Request) *RandomRequestSource {
	return &RandomRequestSource{
		reqs:    reqs,
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
		ch:      make(chan Request),
		filter:  filter,
		metrics: metrics,
	}
}

func (s *RandomRequestSource) Name() string {
	return s.name
}

func (r *RandomRequestSource) Chan() <-chan Request {
	return r.ch
}

func (r *RandomRequestSource) Start() error {
	go func() {
		r.metrics.connected.Set(1)
		defer r.metrics.connected.Set(0)
		defer close(r.ch)

		for {
			r.metrics.requestsIncoming.Add(1)
			idx := r.rng.Intn(len(r.reqs))
			req := *r.reqs[idx]
			req.Timestamp = time.Now()

			if r.filter != nil && !r.filter(&req) {
				r.metrics.requestsFiltered.Add(1)
				continue
			}

			select {
			case <-r.done:
				return
			case r.ch <- req:
			}
		}
	}()

	return nil
}

func (r *RandomRequestSource) Stop() {
	close(r.done)
}

func (r *RandomRequestSource) Err() error {
	return nil
}

// NewNginxLogRequestSource reads a stream of requests
// from an nginx formatted access log file and returns a RandomRequestSource
// that will serve the requests at random. Requests are filtered to GET
// and paths /ipfs and /ipns
func NewNginxLogRequestSource(fname string, filter RequestFilter, metrics *RequestSourceMetrics) (*RandomRequestSource, error) {
	var reqs []*Request

	f, err := os.Open(fname)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		pos1 := bytes.IndexRune(line, '"')
		pos2 := bytes.IndexRune(line[pos1+1:], '"')

		fields := strings.SplitN(string(line[pos1+1:pos1+pos2+1]), " ", 3)
		if len(fields) < 3 {
			continue
		}
		if fields[0] != "GET" {
			continue
		}
		if !strings.HasPrefix(fields[1], "/ipfs") && !strings.HasPrefix(fields[1], "/ipns") {
			continue
		}

		reqs = append(reqs, &Request{
			Method: fields[0],
			URI:    fields[1],
		})
	}

	if scanner.Err() != nil {
		return nil, fmt.Errorf("scanner: %w", scanner.Err())
	}

	return NewRandomRequestSource(filter, metrics, reqs), nil
}

var samplePathsIPFS = []string{
	"/ipfs/QmQPeNsJPyVWPFDVHb77w8G42Fvo15z4bG2X8D2GhfbSXc/readme",
	"/ipfs/bafkreifjjcie6lypi6ny7amxnfftagclbuxndqonfipmb64f2km2devei4",
	"/ipfs/QmUa7f9JtJMsqJJ3s3ZXk6WyF4xJLE8FiqYskZGgk8GCDv",
	"/ipfs/QmaQsTLL3nc5dw6wAvaioJSBfd1jhQrA2o6ucFf7XeV74P",
	"/ipfs/QmWS73SiuSK1zZ2nVSmUb2xZfSwHcdgrYkmXn2ELpQ5XRT",
	"/ipfs/QmVxjFRyhmyQaZEtCh7nk2abc7LhFkzhnRX4rcHqCCpikR",
	"/ipfs/QmUiRx71uxfmUE8V3H9sWAsAXoM88KR4eo1ByvvcFNeTLR",
	"/ipfs/QmcS5JZs8X3TdtkEBpHAdUYjdNDqcL7fWQFtQz69mpnu2X",
	"/ipfs/QmfA31fbCWojSmhSGvvfxmxaYCpMoXP95zEQ9sLvBGHNaN",
	"/ipfs/QmR9i9KL3vhhAqTBGj1bPPC7LvkptxrH9RvxJxLN1vvsBE",
	"/ipfs/QmWV8rqZLxs1oQN9jxNWmnT1YdgLwCcscv94VARrhHf1T7",
	"/ipfs/QmamahpFCstMUqHi2qGtVoDnRrsXhid86qsfvoyCTKJqHr",
	"/ipfs/QmWionkqH2B6TXivzBSQeSyBxojaiAFbzhjtwYRrfwd8nH",
	"/ipfs/Qmf93EMrADXAK6CyiSfE8xx45fkMfR3uzKEPCvZC1n2kzb",
	"/ipfs/QmWS73SiuSK1zZ2nVSmUb2xZfSwHcdgrYkmXn2ELpQ5XRT",
	"/ipfs/QmNT23NWCVFFw9ioBjUCMcBXpHTDgr7tKzaj1ckm5UPWT1/ipfs-029.w3c-blockchain-workshop.compressed.pdf",
	"/ipfs/QmR7tiySn6vFHcEjBeZNtYGAFh735PJHfEMdVEycj9jAPy/docs/getting-started",
	"/ipfs/QmNvTjdqEPjZVWCvRWsFJA1vK7TTw1g9JP6we1WBJTRADM",
	"/ipfs/QmNvTjdqEPjZVWCvRWsFJA1vK7TTw1g9JP6we1WBJTRADM/rfc-data/rfc1113.txt",
	"/ipfs/QmNvTjdqEPjZVWCvRWsFJA1vK7TTw1g9JP6we1WBJTRADM/rfc-data/rfc1147.pdf",
	"/ipfs/QmSnuWmxptJZdLJpKRarxBMS2Ju2oANVrgbr2xWbie9b2D/frontend/pages",
	"/ipfs/QmSnuWmxptJZdLJpKRarxBMS2Ju2oANVrgbr2xWbie9b2D/frontend/thumbnails/21027771304_43d7ae4edc_o.jpg._t.jpg",
	"/ipfs/QmSnuWmxptJZdLJpKRarxBMS2Ju2oANVrgbr2xWbie9b2D/frontend/pages/QXBvbGxvIDE1IE1hZ2F6aW5lIDkzL1A=.html",
	"/ipfs/QmNoscE3kNc83dM5rZNUC5UDXChiTdDcgf16RVtFCRWYuU/food/aphrodis.txt",
	"/ipfs/QmNoscE3kNc83dM5rZNUC5UDXChiTdDcgf16RVtFCRWYuU/food/ppbeer.txt",
	"/ipfs/QmNoscE3kNc83dM5rZNUC5UDXChiTdDcgf16RVtFCRWYuU/humor/aclamt.txt",
	"/ipfs/QmVCjhoEFC9vwvaa8bKyJgwAByP4MXSogcyDGoz4Lkc3ox/SUBSITES/ar.geocities.com.7z.009",
	"/ipfs/QmVCjhoEFC9vwvaa8bKyJgwAByP4MXSogcyDGoz4Lkc3ox/SUBSITES/de.geocities.com.7z.066",
	"/ipfs/QmVCjhoEFC9vwvaa8bKyJgwAByP4MXSogcyDGoz4Lkc3ox/GEOCITIES/www.geocities.com.7z.011",
}

var samplePathsIPNS = []string{
	"/ipns/proofs.filecoin.io/v28-proof-of-spacetime-fallback-merkletree-poseidon_hasher-8-0-0-0170db1f394b35d995252228ee359194b13199d259380541dc529fb0099096b0.meta",
	"/ipns/proofs.filecoin.io/v28-proof-of-spacetime-fallback-merkletree-poseidon_hasher-8-0-0-0170db1f394b35d995252228ee359194b13199d259380541dc529fb0099096b0.params",
	"/ipns/proofs.filecoin.io/v28-proof-of-spacetime-fallback-merkletree-poseidon_hasher-8-0-0-0170db1f394b35d995252228ee359194b13199d259380541dc529fb0099096b0.vk",
	"/ipns/proofs.filecoin.io/v28-proof-of-spacetime-fallback-merkletree-poseidon_hasher-8-0-0-0cfb4f178bbb71cf2ecfcd42accce558b27199ab4fb59cb78f2483fe21ef36d9.meta",
	"/ipns/proofs.filecoin.io/v28-proof-of-spacetime-fallback-merkletree-poseidon_hasher-8-0-0-0cfb4f178bbb71cf2ecfcd42accce558b27199ab4fb59cb78f2483fe21ef36d9.params",
	"/ipns/proofs.filecoin.io/v28-proof-of-spacetime-fallback-merkletree-poseidon_hasher-8-0-0-0cfb4f178bbb71cf2ecfcd42accce558b27199ab4fb59cb78f2483fe21ef36d9.vk",
	"/ipns/en.wikipedia-on-ipfs.org/wiki/United_Kingdom",
	"/ipns/en.wikipedia-on-ipfs.org/wiki/Rugby_School",
	"/ipns/en.wikipedia-on-ipfs.org/wiki/John_Locke",
	"/ipns/en.wikipedia-on-ipfs.org/wiki/Vertigo_(film)",
	"/ipns/en.wikipedia-on-ipfs.org/wiki/Fleetwood_Mac",
	"/ipns/QmYoQ4Gn9vAcimaXT5xWYAPrBCu3QZyLmEvhLFu9djNZCy/whitelist.txt",
	"/ipns/ipfs-planets.echox.app/mainnet/GQJqkw49LrbLAKa/480/echox-nft.gif",
	"/ipns/ipfs-planets.echox.app/mainnet/GQJqkw49LrbLAKa/66/metadata.json",
	"/ipns/fromthemachine.org/ARTIMESIAN.html",
}

func sampleRequests() []*Request {
	paths := []string{}
	paths = append(paths, samplePathsIPFS...)
	paths = append(paths, samplePathsIPNS...)
	return permutePaths(paths)
}

func permuteSamplePathsIPFS() []*Request {
	return permutePaths(samplePathsIPFS)
}

func permuteSamplePathsIPNS() []*Request {
	return permutePaths(samplePathsIPNS)
}

func permutePaths(paths []string) []*Request {
	headerVariants := []map[string]string{
		{},
		{"Accept": "application/vnd.ipld.car"},
		{"Accept": "application/vnd.ipld.raw"},
	}

	reqs := make([]*Request, 0, len(paths)*len(headerVariants))
	for _, p := range paths {
		for _, h := range headerVariants {
			req := &Request{
				Method: "GET",
				URI:    p,
				Header: map[string]string{},
			}

			for k, v := range h {
				req.Header[k] = v
			}

			reqs = append(reqs, req)
		}
	}

	return reqs
}

// LokiRequestSource is a request source that reads a stream of nginx logs from Loki
type LokiRequestSource struct {
	cfg                     LokiConfig
	ch                      chan Request
	done                    chan struct{}
	filter                  RequestFilter
	requestsDroppedCounter  *prometheus.CounterVec
	requestsFilteredCounter *prometheus.CounterVec
	requestsIncomingCounter *prometheus.CounterVec
	errorCounter            *prometheus.CounterVec
	metrics                 *RequestSourceMetrics

	mu   sync.Mutex // guards following fields
	conn *websocket.Conn
	err  error
}

type LokiConfig struct {
	URI      string // URI of the loki server, e.g. https://logs-prod-us-central1.grafana.net
	Username string // For grafana cloud this is a numeric user id
	Password string // For grafana cloud this is the API token
	Query    string // the query to use to obtain logs
}

func NewLokiRequestSource(cfg *LokiConfig, filter RequestFilter, metrics *RequestSourceMetrics, rps int) (*LokiRequestSource, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config must not be nil")
	}
	l := &LokiRequestSource{
		cfg:     *cfg,
		ch:      make(chan Request, rps*60*30), // buffer at least 30 minutes of requests
		done:    make(chan struct{}, 0),
		filter:  filter,
		metrics: metrics,
	}

	return l, nil
}

func (l *LokiRequestSource) Name() string {
	return "loki"
}

func (l *LokiRequestSource) Chan() <-chan Request {
	return l.ch
}

func (l *LokiRequestSource) Start() error {
	client := &client.DefaultClient{
		TLSConfig: config.TLSConfig{},
		Address:   l.cfg.URI,
		Username:  l.cfg.Username,
		Password:  l.cfg.Password,
	}

	q := &query.Query{
		Limit:       100,
		QueryString: l.cfg.Query,
		Start:       time.Now().Add(-60 * time.Minute),
		End:         time.Now(),
		BatchSize:   1000,
	}

	if err := l.connect(client, q); err != nil {
		return err
	}
	defer l.metrics.connected.Set(0)

	type logline struct {
		Server  string            `json:"server"`
		Time    time.Time         `json:"time"`
		Method  string            `json:"method"`
		URI     string            `json:"uri"`
		Status  int               `json:"status"`
		Headers map[string]string `json:"headers"`
	}

	go func() {
		defer close(l.ch)

		for {
			tr, ok := l.readTailResponse()
			if !ok {
				log.Printf("failed to read tail from loki: %v", l.err)
				if err := l.connect(client, q); err != nil {
					l.metrics.errors.Add(1)
					log.Printf("failed to connect to loki: %v", err)
					time.Sleep(30 * time.Second)
				}
				continue
			}
			for _, stream := range tr.Streams {
				for _, entry := range stream.Entries {
					l.metrics.requestsIncoming.Add(1)

					var line logline
					err := json.Unmarshal([]byte(entry.Line), &line)
					if err != nil {
						l.metrics.errors.Add(1)
						continue
					}

					req := Request{
						Method:    line.Method,
						URI:       line.URI,
						Header:    line.Headers,
						Timestamp: line.Time,
					}

					if l.filter != nil && !l.filter(&req) {
						l.metrics.requestsFiltered.Add(1)
						continue
					}

					select {
					case <-l.done:
						return
					case l.ch <- req:

					default:
						l.metrics.requestsDropped.Add(1)
					}

				}
			}

		}
	}()

	return nil
}

func (l *LokiRequestSource) connect(c *client.DefaultClient, q *query.Query) error {
	// Clean up if we were previously connected
	l.mu.Lock()
	if l.conn != nil {
		l.conn.Close()
	}
	l.conn = nil
	l.metrics.connected.Set(0)
	l.mu.Unlock()

	conn, err := c.LiveTailQueryConn(q.QueryString, 0, q.Limit, q.Start, q.Quiet)
	if err != nil {
		l.err = fmt.Errorf("tailing logs failed: %w", err)
		return l.err
	}
	l.mu.Lock()
	l.conn = conn
	l.mu.Unlock()
	l.metrics.connected.Set(1)
	log.Printf("connected to loki: %s", l.cfg.URI)
	return nil
}

func (l *LokiRequestSource) Stop() {
	close(l.done)

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.conn == nil {
		return
	}

	if err := l.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")); err != nil {
		l.err = err
	}
	l.conn = nil
}

func (l *LokiRequestSource) Err() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.err
}

func (l *LokiRequestSource) readTailResponse() (*loghttp.TailResponse, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.conn == nil {
		l.err = fmt.Errorf("disconnected")
		return nil, false
	}

	tr := new(loghttp.TailResponse)
	err := unmarshal.ReadTailResponseJSON(tr, l.conn)
	if err != nil {
		l.err = fmt.Errorf("read tail response json: %w", err)
		return nil, false
	}

	return tr, true
}

type SQSConfig struct {
	AWSConfig *aws.Config
	Queue     string
}

type SQSRequestSource struct {
	cfg     SQSConfig
	ch      chan Request
	done    chan struct{}
	filter  RequestFilter
	metrics *RequestSourceMetrics

	mu       sync.Mutex
	svc      *sqs.SQS
	queueURL string
	err      error
}

func NewSQSRequestSource(cfg *SQSConfig, filter RequestFilter, metrics *RequestSourceMetrics, rps int) (*SQSRequestSource, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config must not be nil")
	}
	s := &SQSRequestSource{
		cfg:     *cfg,
		ch:      make(chan Request, rps*60*30), // buffer at least 30 minutes of requests
		done:    make(chan struct{}, 0),
		filter:  filter,
		metrics: metrics,
	}

	return s, nil
}

func (s *SQSRequestSource) Name() string {
	return "sns"
}

func (s *SQSRequestSource) Chan() <-chan Request {
	return s.ch
}

func (s *SQSRequestSource) Start() error {
	sess, err := session.NewSession(s.cfg.AWSConfig)
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.svc = sqs.New(sess)

	urlResult, err := s.svc.GetQueueUrl(&sqs.GetQueueUrlInput{
		QueueName: aws.String(s.cfg.Queue),
	})
	if err != nil {
		return fmt.Errorf("get queue url: %w", err)
	}
	s.queueURL = *urlResult.QueueUrl
	log.Printf("found queue url %s", s.queueURL)
	s.metrics.connected.Set(1)
	defer s.metrics.connected.Set(0)
	go func() {
		for {
			select {
			case <-s.done:
				return
			default:
			}

			msgResult, err := s.svc.ReceiveMessage(&sqs.ReceiveMessageInput{
				AttributeNames: []*string{
					aws.String(sqs.MessageSystemAttributeNameSentTimestamp),
				},
				MessageAttributeNames: []*string{
					aws.String(sqs.QueueAttributeNameAll),
				},
				QueueUrl:            aws.String(s.queueURL),
				MaxNumberOfMessages: aws.Int64(10),
				VisibilityTimeout:   aws.Int64(5),
				WaitTimeSeconds:     aws.Int64(10),
			})
			if err != nil {
				s.metrics.errors.Add(1)
				log.Printf("failed to receive message: %v", err)
				continue
			}

			for _, msg := range msgResult.Messages {
				if msg.Body == nil {
					s.metrics.errors.Add(1)
					log.Printf("message body was nil: %s", *msg.MessageId)
					continue
				}
				_, err = s.svc.DeleteMessage(&sqs.DeleteMessageInput{
					QueueUrl:      aws.String(s.queueURL),
					ReceiptHandle: msg.ReceiptHandle,
				})
				if err != nil {
					s.metrics.errors.Add(1)
					log.Printf("failed to delete message: %v", err)
				}

				var smsg SNSMessage
				if err := json.Unmarshal([]byte(*msg.Body), &smsg); err != nil {
					s.metrics.errors.Add(1)
					log.Printf("failed to unmarshal message: %v", err)
					continue
				}

				scanner := bufio.NewScanner(strings.NewReader(smsg.Message))
				for scanner.Scan() {
					s.metrics.requestsIncoming.Add(1)
					data := scanner.Bytes()
					var req Request
					if err := json.Unmarshal(data, &req); err != nil {
						s.metrics.errors.Add(1)
						log.Printf("failed to unmarshal request: %v", err)
						continue
					}

					if s.filter != nil && !s.filter(&req) {
						s.metrics.requestsFiltered.Add(1)
						continue
					}

					select {
					case <-s.done:
						return
					case s.ch <- req:
					default:
						s.metrics.requestsDropped.Add(1)
					}
				}

			}

		}
	}()

	return nil
}

func (s *SQSRequestSource) Stop() {
	close(s.done)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.svc == nil {
		return
	}

	s.svc = nil
}

func (s *SQSRequestSource) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

type SNSMessage struct {
	Type      string `json:"Type"`
	MessageId string `json:"MessageId"`
	TopicArn  string `json:"TopicArn"`
	Message   string `json:"Message"`
}
