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
	"github.com/mum4k/termdash/widgets/linechart"
)

type Gui struct {
	source   RequestSource
	backends []*Backend
	cancel   func()
	term     terminalapi.Terminal
	chart    *linechart.LineChart

	cancelLoaderMu sync.Mutex
	cancelLoader   func()
}

func NewGui(source RequestSource, backends []*Backend) (*Gui, error) {
	g := &Gui{
		source:   source,
		backends: backends,
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
		return nil, fmt.Errorf("new linechart: %w", err)
	}

	return g, nil
}

func (g *Gui) Close() {
	g.term.Close()
}

func (g *Gui) Show(ctx context.Context, redrawInterval time.Duration) error {
	c, err := container.New(g.term, container.ID("root"))
	if err != nil {
		return fmt.Errorf("failed to generate container: %w", err)
	}

	row1 := grid.RowHeightPercWithOpts(70,
		[]container.Option{container.ID("ttfb")},
		grid.Widget(g.chart, container.Border(linestyle.Light), container.BorderTitle("TTFB (ms)")),
	)

	builder := grid.New()
	builder.Add(
		row1,
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

	coll := NewCollector(timings, 100*time.Millisecond)
	go coll.Run(ctx)

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

	if err := l.Send(ctx); err != nil {
		if !errors.Is(err, context.Canceled) {
			log.Printf("loader error: %v", err)
		}
	}
	close(timings)
}
