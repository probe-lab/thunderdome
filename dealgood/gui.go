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
	source  RequestSource
	targets []*Target
	cancel  func()
	term    terminalapi.Terminal

	// widgets
	chart         *linechart.LineChart
	progressGauge *gauge.Gauge
	infoText      *text.Text
	keysText      *text.Text
	durationText  *text.Text
	beStatsTexts  map[string]*text.Text
	beColors      map[string]cell.Color

	cancelLoaderMu sync.Mutex
	cancelLoader   func()

	infoMu             sync.Mutex // guards changes to following
	experimentName     string
	duration           int
	durationsIdx       int
	requestRate        int
	requestRateIdx     int
	requestConcurrency int
	concurrenciesIdx   int
	statsFormatterIdx  int
	statsFormatter     *StatsFormatter

	// latestSamples held here so we can view them even when experiment is stopped
	latestSamplesMu sync.Mutex
	latestSamples   map[string]MetricSample
}

var durations = []int{
	-1,
	1 * 60,
	5 * 60,
	10 * 60,
	15 * 60,
	30 * 60,
	60 * 60,
	180 * 60,
	720 * 60,
}

var requestRates = []int{
	1,
	2,
	4,
	8,
	10,
	15,
	20,
	30,
	40,
	50,
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
	10,
	15,
	20,
	30,
	40,
	50,
	60,
	80,
}

var colors = []cell.Color{
	cell.ColorWhite,
	cell.ColorRed,
	cell.ColorLime,
	cell.ColorTeal,
	cell.ColorMaroon,
	cell.ColorGreen,
	cell.ColorOlive,
	cell.ColorNavy,
	cell.ColorPurple,
	cell.ColorSilver,
	cell.ColorGray,
	cell.ColorAqua,
	cell.ColorYellow,
	cell.ColorBlue,
	cell.ColorFuchsia,
}

func NewGui(source RequestSource, exp *Experiment) (*Gui, error) {
	g := &Gui{
		source:             source,
		beStatsTexts:       map[string]*text.Text{},
		beColors:           map[string]cell.Color{},
		experimentName:     exp.Name,
		duration:           exp.Duration,
		requestRate:        exp.Rate,
		requestConcurrency: exp.Concurrency,
		statsFormatter:     &statsFormatters[0],
		targets:            exp.Targets,
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

	for _, be := range g.targets {
		t, err := text.New(text.RollContent())
		if err != nil {
			return nil, fmt.Errorf("target text: %w", err)
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

	for i, be := range g.targets {
		t, ok := g.beStatsTexts[be.Name]
		if !ok {
			continue
		}
		color := colors[i%len(colors)]
		g.beColors[be.Name] = color

		elems = append(elems, grid.ColWidthFixed(24, grid.Widget(t, container.Border(linestyle.Light), container.BorderTitle(be.Name), container.TitleColor(color))))
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
		return fmt.Errorf("build gui: %w", err)
	}

	if err := c.Update("root", gridOpts...); err != nil {
		return fmt.Errorf("failed to update container: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	g.cancel = cancel
	defer g.cancel()

	if err := termdash.Run(ctx, g.term, c, termdash.KeyboardSubscriber(g.OnKey), termdash.RedrawInterval(redrawInterval)); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	return nil
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
			g.StartLoader(ctx)
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
		g.redrawMetrics()
	case 'M': // cycle displayed metric
		g.infoMu.Lock()
		g.statsFormatterIdx--
		if g.statsFormatterIdx < 0 {
			g.statsFormatterIdx = len(statsFormatters) - 1
		}
		g.statsFormatter = &statsFormatters[g.statsFormatterIdx]
		g.infoMu.Unlock()
		g.updateInfoText()
		g.redrawMetrics()
	case 'd': // cycle duration
		g.infoMu.Lock()
		g.durationsIdx = (g.durationsIdx + 1) % len(durations)
		g.duration = durations[g.durationsIdx]
		g.infoMu.Unlock()
		g.updateInfoText()
	case 'D': // cycle duration
		g.infoMu.Lock()
		g.durationsIdx--
		if g.durationsIdx < 0 {
			g.durationsIdx = len(durations) - 1
		}
		g.duration = durations[g.durationsIdx]
		g.infoMu.Unlock()
		g.updateInfoText()
	case 'r': // cycle rate
		g.infoMu.Lock()
		g.requestRateIdx = (g.requestRateIdx + 1) % len(requestRates)
		g.requestRate = requestRates[g.requestRateIdx]
		g.infoMu.Unlock()
		g.updateInfoText()
	case 'R': // cycle rate
		g.infoMu.Lock()
		g.requestRateIdx--
		if g.requestRateIdx < 0 {
			g.requestRateIdx = len(requestRates) - 1
		}
		g.requestRate = requestRates[g.requestRateIdx]
		g.infoMu.Unlock()
		g.updateInfoText()
	case 'c': // cycle concurrency
		g.infoMu.Lock()
		g.concurrenciesIdx = (g.concurrenciesIdx + 1) % len(concurrencies)
		g.requestConcurrency = concurrencies[g.concurrenciesIdx]
		g.infoMu.Unlock()
		g.updateInfoText()
	case 'C': // cycle concurrency
		g.infoMu.Lock()
		g.concurrenciesIdx--
		if g.concurrenciesIdx < 0 {
			g.concurrenciesIdx = len(concurrencies) - 1
		}
		g.requestConcurrency = concurrencies[g.concurrenciesIdx]
		g.infoMu.Unlock()
		g.updateInfoText()
	}
}

func (g *Gui) StartLoader(ctx context.Context) error {
	timings := make(chan *RequestTiming, 10000)

	coll, err := NewCollector(timings, 100*time.Millisecond)
	if err != nil {
		return fmt.Errorf("new collector: %w", err)
	}

	g.infoMu.Lock()
	l, err := NewLoader(g.experimentName, g.targets, g.source, timings, g.requestRate, g.requestConcurrency, g.duration)
	g.infoMu.Unlock()
	if err != nil {
		return fmt.Errorf("new loader: %w", err)
	}

	go coll.Run(ctx)

	go g.Update(ctx, coll)

	go func() {
		defer func() { close(timings) }()

		if err := l.Send(ctx); err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				fmt.Fprintf(os.Stderr, "loader stopped: %v", err)
			}
		}
	}()

	return nil
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

			if duration != -1 {
				// Update the progress indicator
				passed := now.Sub(start)
				percent := int(float64(passed) / float64(time.Duration(duration)*time.Second) * 100)
				if percent > 100 {
					continue
				}
				g.progressGauge.Percent(percent)
			}

			g.updateInfoText()

			latest := coll.Latest()

			g.latestSamplesMu.Lock()
			g.latestSamples = latest
			g.latestSamplesMu.Unlock()

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
				g.chart.Series(name, ch.TTFBP99, linechart.SeriesCellOpts(cell.FgColor(g.beColors[name])))

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

	g.infoText.Write(fmt.Sprintf("%v", durationDesc(g.duration)))
	g.infoText.Write("  Rate: ", text.WriteCellOpts(cell.FgColor(cell.ColorBlue)))
	g.infoText.Write(fmt.Sprintf("%d/s", g.requestRate))
	g.infoText.Write("  Concurrency: ", text.WriteCellOpts(cell.FgColor(cell.ColorBlue)))
	g.infoText.Write(fmt.Sprintf("%d", g.requestConcurrency))
}

func (g *Gui) redrawMetrics() {
	g.latestSamplesMu.Lock()
	latest := g.latestSamples
	g.latestSamplesMu.Unlock()

	g.infoMu.Lock()
	formatter := statsFormatters[g.statsFormatterIdx]
	g.infoMu.Unlock()

	for name, t := range g.beStatsTexts {
		st, ok := latest[name]
		if !ok {
			continue
		}

		formatter.Fn(name, &st, t)
	}
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
				formatStatLineInt(t, "Timeouts", s.TotalTimeoutErrors),
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
	{
		Name: "Request failure rates",
		Fn: func(name string, s *MetricSample, t *text.Text) {
			writeStat(t, name,
				formatStatLineFloat(t, "Conn Err %", 100*(float64(s.TotalConnectErrors)/float64(s.TotalRequests))),
				formatStatLineFloat(t, "Timeout %", 100*(float64(s.TotalTimeoutErrors)/float64(s.TotalRequests))),
				formatStatLineFloat(t, "Dropped %", 100*(float64(s.TotalDropped)/float64(s.TotalRequests))),
				formatStatLineFloat(t, "Serv. Err %", 100*(float64(s.TotalHttp5XX)/float64(s.TotalRequests))),
			)
		},
	},
}

func writeStat(t *text.Text, title string, lines ...string) {
	t.Write(strings.Join(lines, "\n"), text.WriteReplace())
}

func formatStatLineFloat(t *text.Text, label string, value float64) string {
	return fmt.Sprintf("%-11s %9.3f", label, value)
}

func formatStatLineInt(t *text.Text, label string, value int) string {
	return fmt.Sprintf("%-11s % 8d", label, value)
}

func durationDesc(d int) string {
	if d == -1 {
		return "forever"
	}

	s := (time.Duration(d) * time.Second).String()
	if strings.HasSuffix(s, "m0s") {
		s = s[:len(s)-2]
	}
	if strings.HasSuffix(s, "h0m") {
		s = s[:len(s)-2]
	}
	return s
}
