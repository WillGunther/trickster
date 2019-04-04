/**
* Copyright 2018 Comcast Cable Communications Management, LLC
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You may obtain a copy of the License at
* http://www.apache.org/licenses/LICENSE-2.0
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
 */

package main

import (
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"go.opencensus.io/exporter/jaeger"
	"go.opencensus.io/trace"
)

type closer func()

// newTracer initializes the Open Census configuration with the requested
// exporter -- Jaeger or none. As well as a function to flush the
// traces before shutting down.
func newTracer(cfg TracingConfig, logger log.Logger) (closer, error) {

	var traceConfig trace.Config
	switch cfg.SamplerType {
	case "Always":
		traceConfig.DefaultSampler = trace.AlwaysSample()
	case "Probability":
		traceConfig.DefaultSampler = trace.ProbabilitySampler(cfg.SamplerFraction)
	case "Never":
		fallthrough
	default:
		traceConfig.DefaultSampler = trace.NeverSample()
	}

	trace.ApplyConfig(traceConfig)

	if cfg.Exporter == "Jaeger" {
		exporter, err := newJaegerExporter(cfg.Jaeger, logger)
		if err != nil {
			return nil, err
		}
		trace.RegisterExporter(exporter)
		return func() { exporter.Flush() }, nil
	}
	return nil, nil
}

func newJaegerExporter(cfg JaegerConfig, logger log.Logger) (*jaeger.Exporter, error) {

	tags := make([]jaeger.Tag, 0)
	for k, v := range cfg.Tags {
		tags = append(tags, jaeger.StringTag(k, v))
	}

	onError := func(err error) {
		level.Warn(logger).Log("msg", err)
	}

	return jaeger.NewExporter(
		jaeger.Options{
			CollectorEndpoint: cfg.CollectorEndpoint,
			AgentEndpoint:     cfg.AgentEndpoint,
			OnError:           onError,
			Username:          cfg.Username,
			Password:          cfg.Password,
			Process: jaeger.Process{
				ServiceName: cfg.Process,
				Tags:        tags,
			},
			BufferMaxCount: cfg.BufferMaxCount,
		},
	)
}
