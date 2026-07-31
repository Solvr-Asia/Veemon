package config

import (
	"context"

	"veemon-common/monitoring/telemetry"
)

func NewTelemetry(ctx context.Context, cfg *Config) (*telemetry.Telemetry, error) {
	return telemetry.New(ctx, telemetry.Config{
		ServiceName:  cfg.OTelServiceName,
		Environment:  cfg.Environment,
		Endpoint:     cfg.OTelEndpoint,
		ExporterType: cfg.OTelExporterType,
		SampleRatio:  cfg.OTelSampleRatio,
		Enabled:      cfg.OTelEnabled,
	})
}
