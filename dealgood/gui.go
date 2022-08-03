package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/mum4k/termdash"
	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/container"
	"github.com/mum4k/termdash/container/grid"
	"github.com/mum4k/termdash/keyboard"
	"github.com/mum4k/termdash/linestyle"
	"github.com/mum4k/termdash/terminal/tcell"
	"github.com/mum4k/termdash/terminal/termbox"
	"github.com/mum4k/termdash/terminal/terminalapi"
	"github.com/mum4k/termdash/widgets/gauge"
	"github.com/mum4k/termdash/widgets/linechart"
	"github.com/mum4k/termdash/widgets/text"
)

type Gui struct {
	source   RequestSource
	backends []*Backend
	cancel   func()
	term     terminalapi.Terminal

	// widgets
	chart         *linechart.LineChart
	progressGauge *gauge.Gauge
	infoText      *text.Text
	keysText      *text.Text
	durationText  *text.Text
	beStatsTexts  map[string]*text.Text

	cancelLoaderMu sync.Mutex
	cancelLoader   func()

	infoMu             sync.Mutex // guards changes to following
	experimentName     string
	duration           time.Duration
	durationsIdx       int
	requestRate        int
	requestRateIdx     int
	requestConcurrency int
	concurrenciesIdx   int
	statsFormatterIdx  int
	statsFormatter     *StatsFormatter
}

var durations = []time.Duration{
	1 * time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	60 * time.Minute,
	180 * time.Minute,
	720 * time.Minute,
}

var requestRates = []int{
	10,
	20,
	60,
	80,
	100,
	200,
	400,
	600,
	800,
	1000,
	1200,
	1400,
}

var concurrencies = []int{
	1,
	2,
	4,
	8,
	12,
	16,
	20,
	30,
	40,
	50,
	60,
	80,
}

func NewGui(source RequestSource, exp *ExperimentJSON) (*Gui, error) {
	g := &Gui{
		source:             source,
		beStatsTexts:       map[string]*text.Text{},
		experimentName:     exp.Name,
		duration:           time.Duration(exp.Duration) * time.Second,
		requestRate:        exp.Rate,
		requestConcurrency: exp.Concurrency,
		statsFormatter:     &statsFormatters[0],
	}

	for _, be := range exp.Backends {
		g.backends = append(g.backends, &Backend{
			Name:    be.Name,
			BaseURL: be.BaseURL,
			Host:    be.Host,
		})
	}

	// Find the closest indices for the cycle-able values
	for i, v := range durations {
		if v == g.duration {
			g.durationsIdx = i
			break
		} else if i != 0 && v > g.duration {
			g.durationsIdx = i - 1
			break
		}
	}

	for i, v := range requestRates {
		if v == g.requestRate {
			g.requestRateIdx = i
			break
		} else if i != 0 && v > g.requestRate {
			g.requestRateIdx = i - 1
			break
		}
	}

	for i, v := range concurrencies {
		if v == g.requestConcurrency {
			g.concurrenciesIdx = i
			break
		} else if i != 0 && v > g.requestConcurrency {
			g.concurrenciesIdx = i - 1
			break
		}
	}

	var err error
	if runtime.GOOS == "windows" {
		g.term, err = tcell.New()
	} else {
		g.term, err = termbox.New(termbox.ColorMode(terminalapi.ColorMode256))
	}

	if err != nil {
		return nil, fmt.Errorf("new terminal: %w", err)
	}

	g.chart, err = linechart.New(
		linechart.AxesCellOpts(cell.FgColor(cell.ColorRed)),
		linechart.YLabelCellOpts(cell.FgColor(cell.ColorGreen)),
		linechart.XLabelCellOpts(cell.FgColor(cell.ColorGreen)),
	)
	if err != nil {
		return nil, fmt.Errorf("chart: %w", err)
	}

	g.infoText, err = text.New(text.DisableScrolling())
	if err != nil {
		return nil, fmt.Errorf("keys text: %w", err)
	}
	g.updateInfoText()

	g.keysText, err = text.New(text.DisableScrolling())
	if err != nil {
		return nil, fmt.Errorf("keys text: %w", err)
	}
	g.keysText.Write("q", text.WriteCellOpts(cell.Bold(), cell.FgColor(cell.ColorYellow)))
	g.keysText.Write(":quit ")
	g.keysText.Write("s", text.WriteCellOpts(cell.Bold(), cell.FgColor(cell.ColorYellow)))
	g.keysText.Write(":start/stop ")
	g.keysText.Write("m", text.WriteCellOpts(cell.Bold(), cell.FgColor(cell.ColorYellow)))
	g.keysText.Write(":cycle metrics ")
	g.keysText.Write("d", text.WriteCellOpts(cell.Bold(), cell.FgColor(cell.ColorYellow)))
	g.keysText.Write(":cycle duration ")
	g.keysText.Write("r", text.WriteCellOpts(cell.Bold(), cell.FgColor(cell.ColorYellow)))
	g.keysText.Write(":cycle request rate ")
	g.keysText.Write("c", text.WriteCellOpts(cell.Bold(), cell.FgColor(cell.ColorYellow)))
	g.keysText.Write(":cycle concurrency ")

	g.durationText, err = text.New(text.RollContent(), text.WrapAtWords())
	if err != nil {
		return nil, fmt.Errorf("duration text: %w", err)
	}

	for _, be := range g.backends {
		t, err := text.New(text.RollContent())
		if err != nil {
			return nil, fmt.Errorf("backend text: %w", err)
		}
		g.beStatsTexts[be.Name] = t

	}

	g.progressGauge, err = gauge.New(
		gauge.Border(linestyle.None),
	)
	if err != nil {
		return nil, fmt.Errorf("progress gauge: %w", err)
	}

	return g, nil
}

func (g *Gui) Close() {
	g.term.Close()
}

func (g *Gui) Show(ctx context.Context, redrawInterval time.Duration) error {
	c, err := container.New(g.term, container.ID("root"))
	if err != nil {
		return fmt.Errorf("new container: %w", err)
	}

	elems := []grid.Element{}

	for name, t := range g.beStatsTexts {
		elems = append(elems, grid.ColWidthFixed(22, grid.Widget(t, container.Border(linestyle.Light), container.BorderTitle(name))))
	}

	padText, err := text.New(text.DisableScrolling())
	if err != nil {
		return fmt.Errorf("pad text: %w", err)
	}

	elems = append(elems, grid.ColWidthFixed(1, grid.Widget(padText)))

	builder := grid.New()
	builder.Add(

		grid.RowHeightFixed(1,
			grid.ColWidthFixed(60, grid.Widget(g.infoText)),
		),

		grid.RowHeightFixed(7, elems...),

		grid.RowHeightFixed(1,
			grid.ColWidthFixed(60, grid.Widget(g.keysText)),
		),

		grid.RowHeightFixed(3,
			grid.ColWidthFixed(60, grid.Widget(g.progressGauge, container.Border(linestyle.Light))),
		),

		grid.RowHeightFixed(10,
			grid.Widget(g.chart, container.Border(linestyle.Light), container.BorderTitle("TTFB P90 (ms)")),
		),
	)

	gridOpts, err := builder.Build()
	if err != nil {
		return err
	}

	if err := c.Update("root", gridOpts...); err != nil {
		return fmt.Errorf("failed to update container: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	g.cancel = cancel
	defer g.cancel()

	return termdash.Run(ctx, g.term, c, termdash.KeyboardSubscriber(g.OnKey), termdash.RedrawInterval(redrawInterval))
}

func (g *Gui) OnKey(k *terminalapi.Keyboard) {
	switch k.Key {
	case keyboard.KeyCtrlC, 'q': // Quit
		if g.cancel != nil {
			g.cancel()
		}
	case 's': // Start/stop
		g.cancelLoaderMu.Lock()
		defer g.cancelLoaderMu.Unlock()
		if g.cancelLoader == nil {
			ctx, cancel := context.WithCancel(context.Background())
			g.cancelLoader = cancel
			go g.StartLoader(ctx)
		} else {
			g.cancelLoader()
			g.cancelLoader = nil
		}
	case 'm': // cycle displayed metric
		g.infoMu.Lock()
		g.statsFormatterIdx = (g.statsFormatterIdx + 1) % len(statsFormatters)
		g.statsFormatter = &statsFormatters[g.statsFormatterIdx]
		g.infoMu.Unlock()
		g.updateInfoText()
	case 'd': // cycle duration
		g.infoMu.Lock()
		g.durationsIdx = (g.durationsIdx + 1) % len(durations)
		g.duration = durations[g.durationsIdx]
		g.infoMu.Unlock()
		g.updateInfoText()
	case 'r': // cycle rate
		g.infoMu.Lock()
		g.requestRateIdx = (g.requestRateIdx + 1) % len(requestRates)
		g.requestRate = requestRates[g.requestRateIdx]
		g.infoMu.Unlock()
		g.updateInfoText()
	case 'c': // cycle concurrency
		g.infoMu.Lock()
		g.concurrenciesIdx = (g.concurrenciesIdx + 1) % len(concurrencies)
		g.requestConcurrency = concurrencies[g.concurrenciesIdx]
		g.infoMu.Unlock()
		g.updateInfoText()
	}
}

func (g *Gui) StartLoader(ctx context.Context) {
	timings := make(chan *RequestTiming, 10000)

	g.infoMu.Lock()
	l := &Loader{
		Source:      g.source,
		Backends:    g.backends,
		Rate:        g.requestRate,
		Concurrency: g.requestConcurrency,
		Duration:    g.duration,
		Timings:     timings,
	}
	g.infoMu.Unlock()

	coll := NewCollector(timings, 100*time.Millisecond)
	go coll.Run(ctx)

	go g.Update(ctx, coll)

	if err := l.Send(ctx); err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			fmt.Fprintf(os.Stderr, "loader stopped: %v", err)
		}
	}
	close(timings)
}

// Update updates the gui until the context is canceled
func (g *Gui) Update(ctx context.Context, coll *Collector) {
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()

	chartDataMaxLen := 60
	type chartData struct {
		Requests []float64
		TTFBP99  []float64
	}
	charts := map[string]*chartData{}

	start := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			g.infoMu.Lock()
			formatter := statsFormatters[g.statsFormatterIdx]
			duration := g.duration
			g.infoMu.Unlock()

			// Update the progress indicator
			passed := now.Sub(start)
			percent := int(float64(passed) / float64(duration) * 100)
			if percent > 100 {
				continue
			}
			g.progressGauge.Percent(percent)

			g.updateInfoText()

			latest := coll.Latest()

			for name, t := range g.beStatsTexts {
				st, ok := latest[name]
				if !ok {
					continue
				}

				// Update charts
				ch, ok := charts[name]
				if !ok {
					ch = &chartData{}
				}
				if len(ch.Requests) > chartDataMaxLen {
					ch.Requests = ch.Requests[1:]
					ch.TTFBP99 = ch.TTFBP99[1:]
				}
				ch.Requests = append(ch.Requests, float64(st.TotalRequests))
				ch.TTFBP99 = append(ch.TTFBP99, st.TTFB.P99*1000)
				charts[name] = ch
				g.chart.Series(name, ch.TTFBP99)

				formatter.Fn(name, &st, t)
			}
		}
	}
}

func (g *Gui) updateInfoText() {
	g.infoMu.Lock()
	defer g.infoMu.Unlock()

	g.infoText.Write("Metric: ", text.WriteCellOpts(cell.FgColor(cell.ColorBlue)), text.WriteReplace())
	g.infoText.Write(fmt.Sprintf("%-22s", g.statsFormatter.Name))
	g.infoText.Write("  Experiment: ", text.WriteCellOpts(cell.FgColor(cell.ColorBlue)))
	g.infoText.Write(g.experimentName)
	g.infoText.Write("  Duration: ", text.WriteCellOpts(cell.FgColor(cell.ColorBlue)))
	g.infoText.Write(fmt.Sprintf("%v", g.duration))
	g.infoText.Write("  Rate: ", text.WriteCellOpts(cell.FgColor(cell.ColorBlue)))
	g.infoText.Write(fmt.Sprintf("%d/s", g.requestRate))
	g.infoText.Write("  Concurrency: ", text.WriteCellOpts(cell.FgColor(cell.ColorBlue)))
	g.infoText.Write(fmt.Sprintf("%d", g.requestConcurrency))
}

type StatsFormatter struct {
	Name string
	Fn   func(name string, s *MetricSample, t *text.Text)
}

var statsFormatters = []StatsFormatter{
	{
		Name: "Requests",
		Fn: func(name string, s *MetricSample, t *text.Text) {
			writeStat(t, name,
				formatStatLineInt(t, "Requests", s.TotalRequests),
				formatStatLineInt(t, "Conn Errs", s.TotalConnectErrors),
				formatStatLineInt(t, "Dropped", s.TotalDropped),
				formatStatLineInt(t, "Server Errs", s.TotalHttp5XX),
			)
		},
	},
	{
		Name: "TTFB",
		Fn: func(name string, s *MetricSample, t *text.Text) {
			writeStat(t, name,
				formatStatLineFloat(t, "Mean", s.TTFB.P50*1000),
				formatStatLineFloat(t, "P90", s.TTFB.P90*1000),
				formatStatLineFloat(t, "P99", s.TTFB.P99*1000),
				formatStatLineFloat(t, "Min", s.TTFB.Min*1000),
				formatStatLineFloat(t, "Max", s.TTFB.Max*1000),
			)
		},
	},
	{
		Name: "Connect time",
		Fn: func(name string, s *MetricSample, t *text.Text) {
			writeStat(t, name,
				formatStatLineFloat(t, "Mean", s.ConnectTime.P50*1000),
				formatStatLineFloat(t, "P90", s.ConnectTime.P90*1000),
				formatStatLineFloat(t, "P99", s.ConnectTime.P99*1000),
				formatStatLineFloat(t, "Min", s.ConnectTime.Min*1000),
				formatStatLineFloat(t, "Max", s.ConnectTime.Max*1000),
			)
		},
	},
	{
		Name: "Total time",
		Fn: func(name string, s *MetricSample, t *text.Text) {
			writeStat(t, name,
				formatStatLineFloat(t, "Mean", s.TotalTime.P50*1000),
				formatStatLineFloat(t, "P90", s.TotalTime.P90*1000),
				formatStatLineFloat(t, "P99", s.TotalTime.P99*1000),
				formatStatLineFloat(t, "Min", s.TotalTime.Min*1000),
				formatStatLineFloat(t, "Max", s.TotalTime.Max*1000),
			)
		},
	},
	{
		Name: "HTTP Response Codes",
		Fn: func(name string, s *MetricSample, t *text.Text) {
			writeStat(t, name,
				formatStatLineInt(t, "HTTP 2xx", s.TotalHttp2XX),
				formatStatLineInt(t, "HTTP 3xx", s.TotalHttp3XX),
				formatStatLineInt(t, "HTTP 4xx", s.TotalHttp4XX),
				formatStatLineInt(t, "HTTP 5xx", s.TotalHttp5XX),
			)
		},
	},
}

func writeStat(t *text.Text, title string, lines ...string) {
	t.Write(strings.Join(lines, "\n"), text.WriteReplace())
}

func formatStatLineFloat(t *text.Text, label string, value float64) string {
	return fmt.Sprintf("%-11s %7.4f", label, value)
}

func formatStatLineInt(t *text.Text, label string, value int) string {
	return fmt.Sprintf("%-11s % 7d", label, value)
}
