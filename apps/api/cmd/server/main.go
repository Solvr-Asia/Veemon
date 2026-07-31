// Command server runs the HTTP + gRPC API server.
package main

import (
	"time"

	"veemon/config"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
)

func main() {
	fx.New(
		fx.StopTimeout(30*time.Second),
		fx.WithLogger(func(log *zap.Logger) fxevent.Logger {
			return &fxevent.ZapLogger{Logger: log}
		}),
		config.CommonModule,
		config.ServerModule,
	).Run()
}
