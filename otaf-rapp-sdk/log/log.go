// Copyright 2025-2026 coRAN LABS Private Limited
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package log builds the logger an rApp uses for its whole lifetime.
package log

import (
	"github.com/sirupsen/logrus"

	"github.com/coranlabs/OTAF/otaf-rapp-sdk/errs"
)

func New(level, format string) *logrus.Logger {
	l := logrus.New()

	if format == "json" {
		l.SetFormatter(&logrus.JSONFormatter{TimestampFormat: "2006-01-02T15:04:05.000Z07:00"})
	} else {
		l.SetFormatter(&logrus.TextFormatter{FullTimestamp: true, TimestampFormat: "2006-01-02 15:04:05"})
	}

	if lvl, err := logrus.ParseLevel(level); err == nil {
		l.SetLevel(lvl)
	} else {
		l.SetLevel(logrus.InfoLevel)
		l.WithField("requested", level).Warn("unknown log level, defaulting to info")
	}
	return l
}

// Failure records a failure at the level its severity calls for, with the
// classification attached, so it can be found again without anyone reading the
// message.
//
// A critical failure is still logged at error level. A library that killed the
// process would take that decision away from the rApp, which is the only thing
// that knows whether it can carry on.
func Failure(logger *logrus.Logger, err error, msg string) {
	FailureWith(logger, err, msg, nil)
}

// FailureWith is Failure plus context of the caller's own.
func FailureWith(logger *logrus.Logger, err error, msg string, fields map[string]any) {
	if logger == nil || err == nil {
		return
	}

	entry := logger.WithFields(logrus.Fields(errs.LogFields(err))).WithError(err)
	if len(fields) > 0 {
		entry = entry.WithFields(logrus.Fields(fields))
	}

	switch errs.SeverityOf(err) {
	case errs.SeverityInfo:
		entry.Info(msg)
	case errs.SeverityWarning:
		entry.Warn(msg)
	default:
		entry.Error(msg)
	}
}
