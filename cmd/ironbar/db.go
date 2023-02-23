package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"golang.org/x/exp/slog"
)

type DB struct {
	AwsRegion string
	TableName string
}

type ExperimentRecord struct {
	Name       string
	Start      int64
	End        int64
	Definition string
	Resources  string
}

var ErrNotFound = errors.New("not found")

func (d *DB) RecordExperimentStart(ctx context.Context, rec *ExperimentRecord) error {
	logger := slog.With("experiment", rec.Name)
	logger.Info("recording experiment start")
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(d.AwsRegion),
	})
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}

	svc := dynamodb.New(sess)

	din := &dynamodb.PutItemInput{
		TableName: aws.String(d.TableName),
		Item: map[string]*dynamodb.AttributeValue{
			"name": {
				S: aws.String(rec.Name),
			},
			"start": {
				N: aws.String(strconv.FormatInt(rec.Start, 10)),
			},
			"end": {
				N: aws.String(strconv.FormatInt(rec.End, 10)),
			},
			"definition": {
				S: aws.String(rec.Definition),
			},
			"resources": {
				S: aws.String(rec.Resources),
			},
		},
	}

	if _, err := svc.PutItem(din); err != nil {
		return fmt.Errorf("write item: %w", err)
	}

	return nil
}

func (d *DB) RecordExperimentEnd(ctx context.Context, name string, end int64) error {
	logger := slog.With("experiment", name)
	logger.Info("recording experiment end")
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(d.AwsRegion),
	})
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}

	svc := dynamodb.New(sess)

	in := &dynamodb.UpdateItemInput{
		TableName: aws.String(d.TableName),
		Key: map[string]*dynamodb.AttributeValue{
			"name": {
				S: aws.String(name),
			},
		},
		UpdateExpression: aws.String(`SET #e = :e`),
		ExpressionAttributeNames: map[string]*string{
			"#e": aws.String("end"),
		},
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":e": {
				N: aws.String(strconv.FormatInt(end, 10)),
			},
		},
	}

	if _, err := svc.UpdateItem(in); err != nil {
		return fmt.Errorf("update item: %w", err)
	}

	return nil
}

func (d *DB) RemoveExperiment(ctx context.Context, name string) error {
	logger := slog.With("experiment", name)
	logger.Info("removing experiment")
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(d.AwsRegion),
	})
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}

	svc := dynamodb.New(sess)

	in := &dynamodb.DeleteItemInput{
		TableName: aws.String(d.TableName),
		Key: map[string]*dynamodb.AttributeValue{
			"name": {
				S: aws.String(name),
			},
		},
	}

	if _, err := svc.DeleteItem(in); err != nil {
		return fmt.Errorf("delete item: %w", err)
	}

	return nil
}

func (d *DB) ListExperiments(ctx context.Context) ([]ExperimentRecord, error) {
	slog.Debug("listing experiments")
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(d.AwsRegion),
	})
	if err != nil {
		return nil, fmt.Errorf("new session: %w", err)
	}

	svc := dynamodb.New(sess)

	in := &dynamodb.ScanInput{
		TableName: aws.String(d.TableName),
		ExpressionAttributeNames: map[string]*string{
			"#name":  aws.String("name"),
			"#end":   aws.String("end"),
			"#start": aws.String("start"),
		},
		ProjectionExpression: aws.String("#name,#start,#end,resources"),
	}

	out, err := svc.Scan(in)
	if err != nil {
		return nil, fmt.Errorf("scan items: %w", err)
	}

	var recs []ExperimentRecord
	for _, it := range out.Items {
		var rec ExperimentRecord

		if nameAtt, ok := it["name"]; ok && nameAtt != nil && nameAtt.S != nil && *nameAtt.S != "" {
			rec.Name = *nameAtt.S
			slog.Debug("reading experiment record", "name", rec.Name)
		} else {
			slog.Warn("no name found for item")
			continue
		}

		if startAtt, ok := it["start"]; ok && startAtt != nil && startAtt.N != nil {
			rec.Start, err = strconv.ParseInt(*startAtt.N, 10, 64)
			if err != nil {
				slog.Error("invalid start time", err, "name", rec.Name)
				continue
			}
		} else {
			slog.Warn("no start time found for item", "name", rec.Name)
			continue
		}

		if endAtt, ok := it["end"]; ok && endAtt != nil && endAtt.N != nil {
			rec.End, err = strconv.ParseInt(*endAtt.N, 10, 64)
			if err != nil {
				slog.Error("invalid end time", err, "name", rec.Name)
				continue
			}
		} else {
			slog.Warn("no end time found for item", "name", rec.Name)
			continue
		}

		if resourcesAtt, ok := it["resources"]; ok && resourcesAtt != nil && resourcesAtt.S != nil && *resourcesAtt.S != "" {
			rec.Resources = *resourcesAtt.S
		} else {
			slog.Warn("no resources found for item", "name", rec.Name)
			continue
		}

		recs = append(recs, rec)
	}

	return recs, nil
}

func (d *DB) GetExperiment(ctx context.Context, name string) (*ExperimentRecord, error) {
	slog.Debug("getting experiment")
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(d.AwsRegion),
	})
	if err != nil {
		return nil, fmt.Errorf("new session: %w", err)
	}

	svc := dynamodb.New(sess)

	in := &dynamodb.GetItemInput{
		TableName: aws.String(d.TableName),
		Key: map[string]*dynamodb.AttributeValue{
			"name": {
				S: aws.String(name),
			},
		},

		ExpressionAttributeNames: map[string]*string{
			"#name":  aws.String("name"),
			"#end":   aws.String("end"),
			"#start": aws.String("start"),
		},
		ProjectionExpression: aws.String("#name,#start,#end,resources,definition"),
	}

	out, err := svc.GetItem(in)
	if err != nil {
		if _, ok := err.(*dynamodb.ResourceNotFoundException); ok {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get item: %w", err)
	}

	if out.Item == nil {
		return nil, ErrNotFound
	}

	var rec ExperimentRecord

	if nameAtt, ok := out.Item["name"]; ok && nameAtt != nil && nameAtt.S != nil && *nameAtt.S != "" {
		rec.Name = *nameAtt.S
		slog.Debug("reading experiment record", "name", rec.Name)
	} else {
		slog.Warn("no name found for item")
	}

	if startAtt, ok := out.Item["start"]; ok && startAtt != nil && startAtt.N != nil {
		rec.Start, err = strconv.ParseInt(*startAtt.N, 10, 64)
		if err != nil {
			slog.Error("invalid start time", err, "name", rec.Name)
		}
	} else {
		slog.Warn("no start time found for item", "name", rec.Name)
	}

	if endAtt, ok := out.Item["end"]; ok && endAtt != nil && endAtt.N != nil {
		rec.End, err = strconv.ParseInt(*endAtt.N, 10, 64)
		if err != nil {
			slog.Error("invalid end time", err, "name", rec.Name)
		}
	} else {
		slog.Warn("no end time found for item", "name", rec.Name)
	}

	if resourcesAtt, ok := out.Item["resources"]; ok && resourcesAtt != nil && resourcesAtt.S != nil && *resourcesAtt.S != "" {
		rec.Resources = *resourcesAtt.S
	} else {
		slog.Warn("no resources found for item", "name", rec.Name)
	}

	return &rec, nil
}
