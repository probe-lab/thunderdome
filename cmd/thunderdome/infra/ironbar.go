package infra

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/exp/slog"

	"github.com/plprobelab/thunderdome/cmd/ironbar/api"
	"github.com/plprobelab/thunderdome/pkg/exp"
)

func RegisterExperiment(addr string, e *exp.Experiment, res []api.Resource) func(ctx context.Context) (bool, error) {
	return func(ctx context.Context) (bool, error) {
		def, err := json.Marshal(e)
		if err != nil {
			return false, fmt.Errorf("failed to encode experiment: %w", err)
		}

		start := time.Now().UTC()
		end := start.Add(e.Duration)

		man := api.NewExperimentInput{
			Name:       e.Name,
			Start:      start,
			End:        end,
			Definition: string(def),
			Resources:  res,
		}

		content, err := json.Marshal(man)
		if err != nil {
			return false, fmt.Errorf("failed to encode ironbar manifest: %w", err)
		}

		resp, err := http.Post(fmt.Sprintf("http://%s/experiments", addr), "application/json", bytes.NewReader(content))
		if err != nil {
			slog.Error("failed to post to ironbar service", err)
			return false, nil
		}

		if resp.StatusCode != http.StatusOK {
			errorResp := new(ErrorResponse)
			if err := json.NewDecoder(resp.Body).Decode(errorResp); err != nil {
				return false, fmt.Errorf("failed to send experiment manifest, and failed to decode response: %w", err)
			}

			return false, errorResp
		}

		return true, nil
	}
}

type ErrorResponse struct {
	Err string `json:"err"`
}

func (e *ErrorResponse) Error() string {
	return e.Err
}

func GetExperimentStatus(ctx context.Context, addr string, name string) (*api.ExperimentStatusOutput, error) {
	resp, err := http.Get(fmt.Sprintf("http://%s/experiments/%s/status", addr, name))
	if err != nil {
		return nil, fmt.Errorf("get status: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("experiment not found")
		} else {

			errorResp := new(ErrorResponse)
			if err := json.NewDecoder(resp.Body).Decode(errorResp); err != nil {
				return nil, fmt.Errorf("status check failed, and failed to decode response: %w", err)
			}

			return nil, errorResp
		}
	}

	out := new(api.ExperimentStatusOutput)
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return out, nil
}

func ListExperiments(ctx context.Context, addr string) (*api.ListExperimentsOutput, error) {
	resp, err := http.Get(fmt.Sprintf("http://%s/experiments", addr))
	if err != nil {
		return nil, fmt.Errorf("get experiments: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("experiments not found")
		} else {

			errorResp := new(ErrorResponse)
			if err := json.NewDecoder(resp.Body).Decode(errorResp); err != nil {
				return nil, fmt.Errorf("status check failed, and failed to decode response: %w", err)
			}

			return nil, errorResp
		}
	}

	out := new(api.ListExperimentsOutput)
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return out, nil
}
