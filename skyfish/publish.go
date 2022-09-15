package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/prometheus/client_golang/prometheus"
)

const MaxMessageSize = 256 * 1024 // sns has 256kb max message size

type Publisher struct {
	reqs                <-chan Request
	awscfg              *aws.Config
	topicArn            string
	snsErrorCounter     prometheus.Counter
	processErrorCounter prometheus.Counter
	messagesCounter     prometheus.Counter
	requestsCounter     prometheus.Counter
	connectedGauge      prometheus.Gauge
}

func NewPublisher(awscfg *aws.Config, topicArn string, reqs <-chan Request) (*Publisher, error) {
	p := &Publisher{
		reqs:     reqs,
		awscfg:   awscfg,
		topicArn: topicArn,
	}
	var err error
	p.connectedGauge, err = newPrometheusGauge(
		"publisher_connected",
		"Indicates whether the application is connected to sns.",
	)
	if err != nil {
		return nil, fmt.Errorf("new gauge: %w", err)
	}

	p.snsErrorCounter, err = newPrometheusCounter(
		"publisher_sns_error_total",
		"The total number of errors encountered when publishing requests to sns.",
	)
	if err != nil {
		return nil, fmt.Errorf("new counter: %w", err)
	}

	p.processErrorCounter, err = newPrometheusCounter(
		"publisher_process_error_total",
		"The total number of errors encountered when preparing requests to be published.",
	)
	if err != nil {
		return nil, fmt.Errorf("new counter: %w", err)
	}

	p.messagesCounter, err = newPrometheusCounter(
		"publisher_sns_messages_total",
		"The total number of sns messages published.",
	)
	if err != nil {
		return nil, fmt.Errorf("new counter: %w", err)
	}

	p.requestsCounter, err = newPrometheusCounter(
		"publisher_requests_total",
		"The total number of requests published in messages.",
	)
	if err != nil {
		return nil, fmt.Errorf("new counter: %w", err)
	}

	return p, nil
}

// Run starts running the publisher and blocks until the context is canceled or a fatal
// error is encountered.
func (p *Publisher) Run(ctx context.Context) error {
	sess, err := session.NewSession(p.awscfg)
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}
	log.Printf("connected to sns, publishing to topic %s", p.topicArn)

	p.connectedGauge.Set(1)
	defer p.connectedGauge.Set(0)

	svc := sns.New(sess)

	buf := new(bytes.Buffer)
	buf.Grow(MaxMessageSize)

	requests := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case r, ok := <-p.reqs:
			if !ok {
				return fmt.Errorf("request channel closed")
			}

			data, err := json.Marshal(r)
			if err != nil {
				p.processErrorCounter.Add(1)
				log.Printf("failed to marshal request: %v", err)
				continue
			}
			data = append(data, '\n')

			if len(data) > MaxMessageSize {
				p.processErrorCounter.Add(1)
				log.Printf("request too large to send: %d bytes", len(data))
				continue
			}

			if buf.Len()+len(data) > MaxMessageSize {
				_, err := svc.Publish(&sns.PublishInput{
					Message:  aws.String(buf.String()),
					TopicArn: aws.String(p.topicArn),
				})
				if err != nil {
					p.snsErrorCounter.Add(1)
					log.Printf("failed to publish message: %v", err)
				} else {
					p.messagesCounter.Add(1)
					p.requestsCounter.Add(float64(requests))
					totalRequestsSent.Add(int64(requests))
				}

				buf.Reset()
				requests = 0
			}
			_, err = buf.Write(data)
			if err != nil {
				p.processErrorCounter.Add(1)
				log.Printf("failed to buffer request: %v", err)
				continue
			}
			requests++
		}
	}
}

// Shutdown gracefully shuts down the publisher without interrupting any active
// connections. If the context is canceled the function should return the context error.
func (p *Publisher) Shutdown(ctx context.Context) error {
	return nil
}
