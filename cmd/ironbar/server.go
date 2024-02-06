package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/gorilla/mux"
	"golang.org/x/exp/slog"

	"github.com/probe-lab/thunderdome/cmd/ironbar/api"
	"github.com/probe-lab/thunderdome/pkg/prom"
)

type Server struct {
	db              *DB
	monitorInterval time.Duration
	settle          time.Duration
	awsRegion       string

	upGauge            prom.Gauge
	managedGauge       prom.Gauge
	checkErrorsCounter prom.Counter

	mu      sync.Mutex
	managed map[string]*ManagedResources
}

type ManagedResources struct {
	Name      string
	Start     time.Time
	End       time.Time
	Resources []api.Resource
	Deleted   time.Time
}

func NewServer(ctx context.Context, db *DB, awsRegion string, monitorInterval time.Duration, settle time.Duration) (*Server, error) {
	s := &Server{
		db:              db,
		awsRegion:       awsRegion,
		monitorInterval: monitorInterval,
		settle:          settle,
		managed:         make(map[string]*ManagedResources),
	}

	commonLabels := map[string]string{}
	var err error
	s.upGauge, err = prom.NewPrometheusGauge(
		appName,
		"up",
		"Indicates whether the application is running.",
		commonLabels,
	)
	if err != nil {
		return nil, fmt.Errorf("new gauge: %w", err)
	}

	s.managedGauge, err = prom.NewPrometheusGauge(
		appName,
		"managed_experiments",
		"The total number of active experiments being managed.",
		commonLabels,
	)
	if err != nil {
		return nil, fmt.Errorf("new gauge: %w", err)
	}

	s.checkErrorsCounter, err = prom.NewPrometheusCounter(
		appName,
		"check_errors_total",
		"The total number of errors encountered while checking resources.",
		commonLabels,
	)
	if err != nil {
		return nil, fmt.Errorf("new counter: %w", err)
	}

	return s, nil
}

func (s *Server) Run(ctx context.Context) error {
	if err := s.LoadManagedResources(ctx); err != nil {
		return fmt.Errorf("load managed resources: %w", err)
	}

	go s.MonitorResources(ctx)

	mx := mux.NewRouter()

	s.ConfigureRoutes(mx)

	srv := &http.Server{
		Handler:     mx,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	go func() {
		<-ctx.Done()
		if err := srv.Shutdown(context.Background()); err != nil {
			slog.Error("failed to shut down RPC server", err)
		}
	}()

	slog.Info("starting server", "addr", options.addr)
	listener, err := net.Listen("tcp", options.addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %q: %w", options.addr, err)
	}

	if err := srv.Serve(listener); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve failed: %w", err)
		}
	}

	return nil
}

func (s *Server) LoadManagedResources(ctx context.Context) error {
	slog.Info("loading managed resources")
	recs, err := s.db.ListExperiments(ctx)
	if err != nil {
		return fmt.Errorf("list experiments: %w", err)
	}

	for _, rec := range recs {
		m := new(ManagedResources)
		err := json.Unmarshal([]byte(rec.Resources), &m.Resources)
		if err != nil {
			slog.Error("failed to unmarshal resources", err, "experiment", rec.Name)
			continue
		}

		m.Name = rec.Name
		m.Start = time.Unix(0, rec.Start)
		m.End = time.Unix(0, rec.End)
		slog.Info("found managed resources", "experiment", m.Name, "end", m.End)
		s.managed[m.Name] = m
	}

	return nil
}

func (s *Server) ConfigureRoutes(r *mux.Router) {
	r.NotFoundHandler = http.HandlerFunc(s.NotFoundHandler)
	r.Path("/experiments").Methods("POST").HandlerFunc(s.NewExperimentHandler)
	r.Path("/experiments").Methods("GET").HandlerFunc(s.ListExperimentsHandler)
	r.Path("/experiments/{name}/status").Methods("GET").HandlerFunc(s.ExperimentStatusHandler)
	r.Path("/experiments/{name}").Methods("GET").HandlerFunc(s.GetExperimentHandler)
	r.Path("/experiments/{name}").Methods("DELETE").HandlerFunc(s.DeleteExperimentHandler)
	r.Path("/").Methods("GET").HandlerFunc(s.RootHandler)
}

func (s *Server) MonitorResources(ctx context.Context) {
	slog.Info("starting monitoring of resources", "interval", s.monitorInterval)
	s.upGauge.Set(1)
	defer func() {
		s.upGauge.Set(0)
	}()
	slog.Debug("checking resources")
	s.CheckResources(ctx)

	tick := time.NewTicker(s.monitorInterval)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Debug("stopping monitoring of resources")
			return
		case <-tick.C:
			slog.Debug("checking resources")
			s.CheckResources(ctx)
		}
	}
}

func (s *Server) CheckResources(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(s.awsRegion),
	})
	if err != nil {
		slog.Error("failed to create aws session", err)
		return
	}

	activeManaged := 0
	now := time.Now().UTC()
	for name, mr := range s.managed {
		if !mr.Deleted.IsZero() {
			if time.Since(mr.Deleted) > 24*time.Hour {
				delete(s.managed, name)
			}
			continue
		}
		logger := slog.With("experiment", name)

		if mr.Start.After(now.Add(s.settle)) {
			logger.Info("waiting for experiment to settle before checking resources")
			activeManaged++
			continue
		}

		if mr.End.After(now) {
			logger.Debug("experiment is not due to end yet")
			activeManaged++
			continue
		}

		logger.Info("experiment is due to end")

		anyActive := false
		for _, res := range mr.Resources {
			switch res.Type {
			case api.ResourceTypeEcsTask:
				active, err := isTaskActive(ctx, sess, res.Keys[api.ResourceKeyEcsClusterArn], res.Keys[api.ResourceKeyArn])
				if err != nil {
					logger.Error("failed to check whether task is active", err, "arn", res.Keys[api.ResourceKeyArn], "cluster_arn", res.Keys[api.ResourceKeyEcsClusterArn])
					s.checkErrorsCounter.Add(1)
					continue
				}
				if !active {
					logger.Debug("task is not active")
					continue
				}
				anyActive = true
				logger.Info("task is active, stopping it")
				if err := stopEcsTask(ctx, sess, res.Keys[api.ResourceKeyEcsClusterArn], res.Keys[api.ResourceKeyArn]); err != nil {
					logger.Error("failed to stop task", err, "arn", res.Keys[api.ResourceKeyArn], "cluster_arn", res.Keys[api.ResourceKeyEcsClusterArn])
					s.checkErrorsCounter.Add(1)
				}

			case api.ResourceTypeEcsTaskDefinition:
				active, err := isTaskDefinitionActive(ctx, sess, res.Keys[api.ResourceKeyArn])
				if err != nil {
					logger.Error("failed to check whether task definition is active", err, "arn", res.Keys[api.ResourceKeyArn])
					s.checkErrorsCounter.Add(1)
					continue
				}
				if !active {
					logger.Debug("task definition is not active")
					continue
				}
				anyActive = true
				logger.Info("task definition is active, deregistering it")
				if err := deregisterEcsTaskDefinition(ctx, sess, res.Keys[api.ResourceKeyArn]); err != nil {
					logger.Error("failed to deregister task definition", err, "arn", res.Keys[api.ResourceKeyArn])
					s.checkErrorsCounter.Add(1)
				}

			case api.ResourceTypeEcsSnsSubscription:
				active, err := isSnsSubscriptionActive(ctx, sess, res.Keys[api.ResourceKeyArn])
				if err != nil {
					logger.Error("failed to check whether subscription is active", err, "arn", res.Keys[api.ResourceKeyArn])
					s.checkErrorsCounter.Add(1)
					continue
				}
				if !active {
					logger.Debug("subscription is not active")
					s.checkErrorsCounter.Add(1)
					continue
				}
				anyActive = true
				logger.Info("subscription is active, unsubscribing")
				if err := unsubscribeSqsQueue(ctx, sess, res.Keys[api.ResourceKeyArn]); err != nil {
					logger.Error("failed to unsubscribe queue", err, "arn", res.Keys[api.ResourceKeyArn])
					s.checkErrorsCounter.Add(1)
				}

			case api.ResourceTypeSqsQueue:
				active, err := isSqsQueueActive(ctx, sess, res.Keys[api.ResourceKeyQueueURL])
				if err != nil {
					logger.Error("failed to check whether queue is active", err, "url", res.Keys[api.ResourceKeyQueueURL])
					s.checkErrorsCounter.Add(1)
					continue
				}
				if !active {
					logger.Debug("queue is not active")
					continue
				}
				anyActive = true
				logger.Info("queue is active, deleting")
				if err := deleteSqsQueue(ctx, sess, res.Keys[api.ResourceKeyQueueURL]); err != nil {
					logger.Error("failed to delete queue", err, "url", res.Keys[api.ResourceKeyQueueURL])
					s.checkErrorsCounter.Add(1)
				}

			case api.ResourceTypeEc2Instance:
				_, err := isEc2InstanceActive(ctx, sess, res.Keys[api.ResourceKeyEc2InstanceID])
				if err != nil {
					logger.Error("failed to check whether ec2 instance is active", err, "instance_id", res.Keys[api.ResourceKeyEc2InstanceID])
					s.checkErrorsCounter.Add(1)
					continue
				}

			default:
				anyActive = true
				logger.Warn("unknown resource type, cannot remove", "type", res.Type)
				s.checkErrorsCounter.Add(1)
			}
		}

		if anyActive {
			logger.Info("some resources are still active or stopping, will check again")
			activeManaged++
		} else {
			logger.Info("no resources are active")
			if err := s.db.RemoveExperiment(ctx, name); err != nil {
				logger.Error("failed to remove experiment", err)
				s.checkErrorsCounter.Add(1)
				continue
			}
			mr.Deleted = time.Now().UTC()
		}
	}
	s.managedGauge.Set(float64(activeManaged))
}

func (s *Server) NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte("Not Found\n"))
}

type ErrorResponse struct {
	Err string `json:"err"`
}

func (s *Server) WriteAsJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	if err := enc.Encode(v); err != nil {
		slog.Error("failed to write json response", err)
	}
}

func (s *Server) NotFound(w http.ResponseWriter, r *http.Request, err error) {
	w.Header().Set("Content-Type", "plain/text")
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte("Not found\n"))
}

func (s *Server) BadRequest(w http.ResponseWriter, r *http.Request, err error) {
	slog.Info("bad request", "error", err)
	s.WriteAsJSON(w, http.StatusBadRequest, &ErrorResponse{Err: err.Error()})
}

func (s *Server) ServerError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("server error", err)
	s.WriteAsJSON(w, http.StatusInternalServerError, &ErrorResponse{Err: err.Error()})
}

func (s *Server) NewExperimentHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	in := new(api.NewExperimentInput)

	if err := json.NewDecoder(r.Body).Decode(in); err != nil {
		s.BadRequest(w, r, fmt.Errorf("parse input: %w", err))
		return
	}

	// TODO: what if already managing this experiment

	resJSON, err := json.Marshal(in.Resources)
	if err != nil {
		s.ServerError(w, r, fmt.Errorf("failed to marshal resources: %w", err))
		return
	}

	rec := &ExperimentRecord{
		Name:       in.Name,
		Start:      in.Start.UnixNano(),
		End:        in.End.UnixNano(),
		Definition: in.Definition,
		Resources:  string(resJSON),
	}

	if err := s.db.RecordExperimentStart(ctx, rec); err != nil {
		s.ServerError(w, r, fmt.Errorf("failed to record start of experiment: %w", err))
		return
	}

	s.managed[in.Name] = &ManagedResources{
		Name:      in.Name,
		Start:     in.Start,
		End:       in.End,
		Resources: in.Resources,
	}

	s.WriteAsJSON(w, http.StatusOK, &api.NewExperimentOutput{
		Message:   "Experiment recorded",
		URL:       "/experiments/" + in.Name,
		StatusURL: "/experiments/" + in.Name + "/status",
	})
}

func (s *Server) ListExperimentsHandler(w http.ResponseWriter, r *http.Request) {
	out := &api.ListExperimentsOutput{
		Items: []api.ListExperimentsItem{},
	}

	s.mu.Lock()
	for _, mr := range s.managed {
		out.Items = append(out.Items, api.ListExperimentsItem{
			Name:    mr.Name,
			Start:   mr.Start,
			End:     mr.End,
			Stopped: mr.Deleted,
		})
	}
	s.mu.Unlock()
	s.WriteAsJSON(w, http.StatusOK, out)
}

func (s *Server) ExperimentStatusHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)

	name := vars["name"]
	if len(name) == 0 {
		s.NotFoundHandler(w, r)
		return
	}

	s.mu.Lock()
	mr, ok := s.managed[name]
	s.mu.Unlock()

	if !ok {
		s.NotFoundHandler(w, r)
		return
	}

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(s.awsRegion),
	})
	if err != nil {
		s.ServerError(w, r, fmt.Errorf("failed to create aws session: %w", err))
		return
	}

	out := &api.ExperimentStatusOutput{
		Start:   mr.Start,
		End:     mr.End,
		Stopped: mr.Deleted,
		Status:  "Unknown",
	}

	if !mr.Deleted.IsZero() {
		out.Status = "Stopped"
	} else {

		allActive := true
		receivedErrors := false
		for _, res := range mr.Resources {
			switch res.Type {
			case api.ResourceTypeEcsTask:
				active, err := isTaskActive(ctx, sess, res.Keys[api.ResourceKeyEcsClusterArn], res.Keys[api.ResourceKeyArn])
				if err != nil {
					receivedErrors = true
					continue
				}
				if !active {
					allActive = false
					continue
				}
			case api.ResourceTypeEcsTaskDefinition:
				active, err := isTaskDefinitionActive(ctx, sess, res.Keys[api.ResourceKeyArn])
				if err != nil {
					receivedErrors = true
					continue
				}
				if !active {
					allActive = false
					continue
				}

			case api.ResourceTypeEcsSnsSubscription:
				active, err := isSnsSubscriptionActive(ctx, sess, res.Keys[api.ResourceKeyArn])
				if err != nil {
					receivedErrors = true
					continue
				}
				if !active {
					allActive = false
					continue
				}

			case api.ResourceTypeSqsQueue:
				active, err := isSqsQueueActive(ctx, sess, res.Keys[api.ResourceKeyQueueURL])
				if err != nil {
					receivedErrors = true
					continue
				}
				if !active {
					allActive = false
					continue
				}

			default:
				receivedErrors = true
			}
		}

		if receivedErrors {
			out.Status = "Error"
		} else if allActive {
			out.Status = "Running"
		} else {
			out.Status = "Degraded"
		}
	}
	s.WriteAsJSON(w, http.StatusOK, out)
}

func (s *Server) DeleteExperimentHandler(w http.ResponseWriter, r *http.Request) {
	// ctx := r.Context()
	vars := mux.Vars(r)

	name := vars["name"]
	if len(name) == 0 {
		s.NotFoundHandler(w, r)
		return
	}
}

func (s *Server) RootHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hello, this is ironbar\n"))
}

func (s *Server) GetExperimentHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)

	name := vars["name"]
	if len(name) == 0 {
		s.NotFoundHandler(w, r)
		return
	}

	s.mu.Lock()
	mr, ok := s.managed[name]
	s.mu.Unlock()

	if !ok {
		s.NotFoundHandler(w, r)
		return
	}

	er, err := s.db.GetExperiment(ctx, name)
	if err != nil {
		slog.Error("failed to get experiment", err)
	}

	out := &api.GetExperimentOutput{
		Name:       name,
		Start:      mr.Start,
		End:        mr.End,
		Stopped:    mr.Deleted,
		Definition: er.Definition,
	}
	s.WriteAsJSON(w, http.StatusOK, out)
}
