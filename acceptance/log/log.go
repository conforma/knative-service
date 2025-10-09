// Copyright The Conforma Contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package log

import (
	"context"
	"log/slog"
	"os"
)

type contextKey string

const loggerKey contextKey = "logger"

// Logger wraps slog.Logger with additional functionality for testing
type Logger struct {
	*slog.Logger
	name string
}

// Name sets the logger name for test identification
func (l *Logger) Name(name string) {
	l.name = name
	l.Logger = l.Logger.With("test", name)
}

// LoggerFor creates or retrieves a logger from the context
func LoggerFor(ctx context.Context) (*Logger, context.Context) {
	if logger, ok := ctx.Value(loggerKey).(*Logger); ok {
		return logger, ctx
	}

	// Create a new logger
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	
	logger := &Logger{
		Logger: slog.New(handler),
	}

	return logger, context.WithValue(ctx, loggerKey, logger)
}

