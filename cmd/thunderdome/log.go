package main

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/exp/slog"
)

const (
	colorDefault = "\x1b[0m"
	colorRed     = "\x1b[1;31m"
	colorGreen   = "\x1b[1;32m"
	colorYellow  = "\x1b[1;33m"
	colorBlue    = "\x1b[1;34m"
)

var _ slog.Handler = (*InteractiveHandler)(nil)

type InteractiveHandler struct {
	level     slog.Level
	nocolor   bool
	flatattrs string
	component string
	step      string
}

func NewInteractiveHandler() *InteractiveHandler {
	return &InteractiveHandler{}
}

func (ih *InteractiveHandler) WithLevel(level slog.Level) *InteractiveHandler {
	ih.level = level
	return ih
}

func (ih *InteractiveHandler) WithoutColor() *InteractiveHandler {
	ih.nocolor = true
	return ih
}

func (ih *InteractiveHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= ih.level
}

func (ih *InteractiveHandler) Handle(r slog.Record) error {
	prefix := "???"
	switch r.Level {
	case slog.LevelError:
		prefix = "error"
	case slog.LevelWarn:
		prefix = "warn"
	case slog.LevelInfo:
		prefix = "info"
	case slog.LevelDebug:
		prefix = "debug"
	default:
		prefix = fmt.Sprintf("%02d", r.Level)
	}

	if !ih.nocolor {
		if r.Level >= slog.LevelError {
			prefix = fmt.Sprintf("%s%-5s%s", colorRed, prefix, colorDefault)
		} else if r.Level >= slog.LevelWarn {
			prefix = fmt.Sprintf("%s%-5s%s", colorYellow, prefix, colorDefault)
		} else if r.Level >= slog.LevelInfo {
			prefix = fmt.Sprintf("%s%-5s%s", colorGreen, prefix, colorDefault)
		}
	} else {
		prefix = fmt.Sprintf("%-5s", prefix)
	}

	component := ih.component
	step := ih.step

	var b strings.Builder
	r.Attrs(func(a slog.Attr) {
		if a.Key == "component" {
			component = a.Value.String()
			return
		}
		if a.Key == "step" {
			step = a.Value.String()
			return
		}
		b.WriteString(" ")
		if !ih.nocolor {
			b.WriteString(colorBlue)
		}
		b.WriteString(a.Key)
		if !ih.nocolor {
			b.WriteString(colorDefault)
		}
		b.WriteString("=")
		b.WriteString(quote(a.Value.String()))
	})

	flatattrs := b.String()
	msg := r.Message
	if step != "" {
		msg = step + ": " + msg
	}
	if component != "" {
		msg = component + ": " + msg
	}

	fmt.Printf("%s | %15s | %-40s %s\n", prefix, r.Time.Format("15:04:05.000000"), msg, flatattrs)

	return nil
}

func (ih *InteractiveHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	ih2 := &InteractiveHandler{
		nocolor:   ih.nocolor,
		level:     ih.level,
		component: ih.component,
		step:      ih.step,
	}

	b := new(strings.Builder)
	b.WriteString(ih.flatattrs)
	for _, a := range attrs {
		if a.Key == "component" {
			ih2.component = a.Value.String()
			continue
		}
		if a.Key == "step" {
			ih2.step = a.Value.String()
			continue
		}
		b.WriteString(" ")
		if !ih.nocolor {
			b.WriteString(colorYellow)
		}
		b.WriteString(a.Key)
		if !ih.nocolor {
			b.WriteString(colorDefault)
		}
		b.WriteString("=")
		b.WriteString(quote(a.Value.String()))
	}

	ih2.flatattrs = b.String()

	return ih2
}

func (ih *InteractiveHandler) WithGroup(name string) slog.Handler {
	return ih
}

func quote(s string) string {
	if strings.ContainsAny(s, " ") {
		return fmt.Sprintf("%q", s)
	}
	return s
}
