package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime"
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
	keysText      *text.Text
	durationText  *text.Text
	beStatsTexts  map[string]*text.Text

	cancelLoaderMu sync.Mutex
	cancelLoader   func()
}

func NewGui(source RequestSource, backends []*Backend) (*Gui, error) {
	g := &Gui{
		source:       source,
		backends:     backends,
		beStatsTexts: map[string]*text.Text{},
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

	g.keysText, err = text.New(text.DisableScrolling())
	if err != nil {
		return nil, fmt.Errorf("keys text: %w", err)
	}
	g.keysText.Write("q", text.WriteCellOpts(cell.Bold(), cell.FgColor(cell.ColorYellow)))
	g.keysText.Write(":quit ")
	g.keysText.Write("s", text.WriteCellOpts(cell.Bold(), cell.FgColor(cell.ColorYellow)))
	g.keysText.Write(":start/stop ")

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
		elems = append(elems, grid.ColWidthFixed(60, grid.Widget(t, container.Border(linestyle.Light), container.BorderTitle(name))))
	}

	row1 := grid.RowHeightFixed(6, elems...)

	row2 := grid.RowHeightFixed(1,
		grid.ColWidthFixed(60, grid.Widget(g.keysText)),
	)

	row3 := grid.RowHeightFixed(3,
		grid.ColWidthFixed(60, grid.Widget(g.progressGauge, container.Border(linestyle.Light))),
	)

	row4 := grid.RowHeightFixed(10,
		grid.Widget(g.chart, container.Border(linestyle.Light), container.BorderTitle("TTFB (ms)")),
	)

	builder := grid.New()
	builder.Add(
		row1,
		row2,
		row3,
		row4,
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
	}
}

func (g *Gui) StartLoader(ctx context.Context) {
	timings := make(chan *RequestTiming, 10000)

	l := &Loader{
		// Source: NewStdinRequestSource(),
		Source: NewRandomRequestSource(sampleRequests),
		Backends: []*Backend{
			{
				Name:    "local",
				BaseURL: "http://localhost:8080",
			},
		},
		Rate:        1000, // per second
		Concurrency: 50,   // concurrent requests per backend
		Duration:    60 * time.Second,
		Timings:     timings,
	}

	coll := NewCollector(timings, 100*time.Millisecond)
	go coll.Run(ctx)

	go g.Update(ctx, coll, 60*time.Second)

	if err := l.Send(ctx); err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			log.Printf("loader error: %v", err)
		}
	}
	close(timings)
}

// Update updates the gui until the context is canceled
func (g *Gui) Update(ctx context.Context, coll *Collector, duration time.Duration) {
	t := time.NewTicker(500 * time.Millisecond)
	defer t.Stop()

	chartDataMaxLen := 60
	chartData := []float64{}

	f := func(name string, s *MetricSample, t *text.Text) {
		t.Write(
			fmt.Sprintf("%s\nRequests:  % 7d\nConn Errs: % 7d\nMin: %.4f Max: %.4f P50: %.4f\n P90: %.4f P95: %.4f P99: %.4f",
				name,
				s.TotalRequests,
				s.TotalConnectErrors,
				s.TTFB.Min*1000,
				s.TTFB.Max*1000,
				s.TTFB.P50*1000,
				s.TTFB.P90*1000,
				s.TTFB.P95*1000,
				s.TTFB.P99*1000,
			), text.WriteReplace())
	}

	start := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			// Update the progress indicator
			passed := now.Sub(start)
			percent := int(float64(passed) / float64(duration) * 100)
			if percent > 100 {
				continue
			}
			g.progressGauge.Percent(percent)

			latest := coll.Latest()

			for name, t := range g.beStatsTexts {
				st, ok := latest[name]
				if !ok {
					continue
				}

				if name == "local" {
					if len(chartData) > chartDataMaxLen {
						chartData = chartData[1:]
					}
					chartData = append(chartData, st.TTFB.P99*1000)
					g.chart.Series("TTFB P99", chartData)

				}

				f(name, &st, t)
			}
		}
	}
}
