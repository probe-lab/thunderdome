package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/probe-lab/thunderdome/pkg/filter"
	"github.com/probe-lab/thunderdome/pkg/loki"
	"github.com/probe-lab/thunderdome/pkg/request"
	"github.com/probe-lab/thunderdome/pkg/run"
)

var TailCommand = &cli.Command{
	Name:   "tail",
	Usage:  "Tail logs",
	Action: Tail,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "output",
			Usage:       "Filename that logs should be written to. Use - to write to stdout.",
			Value:       "-",
			Destination: &tailOpts.output,
			EnvVars:     []string{"LOGTOOL_OUTPUT"},
		},
		&cli.StringFlag{
			Name:        "loki-uri",
			Usage:       "URI of the loki server when using loki as a request source.",
			Value:       "",
			Destination: &tailOpts.lokiURI,
			EnvVars:     []string{"LOGTOOL_LOKI_URI"},
		},
		&cli.StringFlag{
			Name:        "loki-username",
			Usage:       "Username to use when using loki as a request source.",
			Value:       "",
			Destination: &tailOpts.lokiUsername,
			EnvVars:     []string{"LOGTOOL_LOKI_USERNAME"},
		},
		&cli.StringFlag{
			Name:        "loki-password",
			Usage:       "Password to use when using loki as a request source.",
			Value:       "",
			Destination: &tailOpts.lokiPassword,
			EnvVars:     []string{"LOGTOOL_LOKI_PASSWORD"},
		},
		&cli.StringFlag{
			Name:        "loki-query",
			Usage:       "Query to use when using loki as a request source.",
			Value:       "",
			Destination: &tailOpts.lokiQuery,
			EnvVars:     []string{"LOGTOOL_LOKI_QUERY"},
		},
		&cli.StringFlag{
			Name:        "filter",
			Usage:       "Filter to apply to requests from the request source (all, pathonly, validpathonly)",
			Value:       "pathonly",
			Destination: &tailOpts.filter,
			EnvVars:     []string{"LOGTOOL_FILTER"},
		},
		&cli.IntFlag{
			Name:        "max-requests",
			Usage:       "Stop tailing once this number of requests have been written.",
			Value:       0,
			Destination: &tailOpts.maxRequests,
			EnvVars:     []string{"LOGTOOL_MAX_REQUESTS"},
		},
		&cli.DurationFlag{
			Name:        "max-time",
			Usage:       "Stop tailing after this interval of time.",
			Value:       0,
			Destination: &tailOpts.maxTime,
			EnvVars:     []string{"LOGTOOL_MAX_TIME"},
		},
	},
}

var tailOpts struct {
	output       string
	lokiURI      string
	lokiUsername string
	lokiPassword string
	lokiQuery    string
	filter       string
	maxRequests  int
	maxTime      time.Duration
}

func Tail(cc *cli.Context) error {
	ctx := cc.Context

	var fltr filter.RequestFilter
	switch tailOpts.filter {
	case "all":
		fltr = filter.NullRequestFilter
	case "pathonly":
		fltr = filter.PathRequestFilter
	case "validpathonly":
		fltr = filter.ValidPathRequestFilter
	default:
		return fmt.Errorf("unsupported filter: %s", tailOpts.filter)
	}

	var output io.Writer
	switch tailOpts.output {
	case "-":
		output = os.Stdout
	default:
		fname := tailOpts.output
		if !strings.HasPrefix(tailOpts.output, "/") {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("could not get working directory: %w", err)
			}

			fname = filepath.Join(cwd, tailOpts.output)
		}
		f, err := os.Create(fname)
		if err != nil {
			return err
		}
		defer f.Close()
		log.Printf("writing requests to %s", fname)
		output = f
	}

	cfg := &loki.LokiConfig{
		AppName:  appName,
		URI:      tailOpts.lokiURI,
		Username: tailOpts.lokiUsername,
		Password: tailOpts.lokiPassword,
		Query:    tailOpts.lokiQuery,
	}

	rg := new(run.Group)

	source, err := loki.NewLokiTailer(cfg)
	if err != nil {
		return fmt.Errorf("loki source: %w", err)
	}
	rg.Add(source)

	publisher, err := NewPrinter(output, source.Chan(), fltr, tailOpts.maxRequests, tailOpts.maxTime)
	if err != nil {
		return fmt.Errorf("new publisher: %w", err)
	}
	rg.Add(publisher)

	return rg.RunAndWait(ctx)
}

type Printer struct {
	w           io.Writer
	logch       <-chan loki.LogLine
	filter      filter.RequestFilter
	maxRequests int
	maxTime     time.Duration
}

func NewPrinter(w io.Writer, logch <-chan loki.LogLine, fltr filter.RequestFilter, maxRequests int, maxTime time.Duration) (*Printer, error) {
	p := &Printer{
		w:           w,
		logch:       logch,
		filter:      fltr,
		maxRequests: maxRequests,
		maxTime:     maxTime,
	}

	return p, nil
}

// Run starts running the printer and blocks until the context is canceled or a fatal
// error is encountered.
func (p *Printer) Run(ctx context.Context) error {
	requests := 0
	written := 0
	filtered := 0

	if p.maxTime > 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, p.maxTime)
		defer cancel()
	}

	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()

	defer func() {
		log.Printf("%d requests seen, %d written, %d excluded by filter\n", requests, written, filtered)
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			log.Printf("%d requests seen, %d written, %d excluded by filter\n", requests, written, filtered)
		case ll, ok := <-p.logch:
			if !ok {
				return fmt.Errorf("request channel closed")
			}
			requests++

			r := request.Request{
				Method:       ll.Method,
				URI:          ll.URI,
				Header:       ll.Headers,
				Status:       ll.Status,
				Timestamp:    ll.Time,
				RemoteAddr:   ll.RemoteAddr,
				UserAgent:    ll.UserAgent,
				Referer:      ll.Referer,
				RespBodySize: ll.RespBodySize,
				RespTime:     ll.RespTime,
			}
			if p.filter != nil && !p.filter(&r) {
				filtered++
				continue
			}

			data, err := json.Marshal(r)
			if err != nil {
				log.Printf("failed to marshal request: %v", err)
				continue
			}
			data = append(data, '\n')

			p.w.Write(data)
			written++
		}

		if p.maxRequests > 0 && requests >= p.maxRequests {
			return fmt.Errorf("maximum number of requests written")
		}

	}
}
