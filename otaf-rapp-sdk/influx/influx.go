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

// Package influx persists the time series an rApp derives, in the store
// deployed beside it by the same rApp package.
package influx

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"

	rappsdk "github.com/coranlabs/OTAF/otaf-rapp-sdk"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/errs"
)

const (
	defaultBuffer = 4096
	batchSize     = 500
	flushEvery    = 5 * time.Second
)

type Config struct {
	URL    string `yaml:"url" env:"INFLUX_URL"`
	Org    string `yaml:"org" env:"INFLUX_ORG"`
	Bucket string `yaml:"bucket" env:"INFLUX_BUCKET"`
	Token  string `yaml:"token" env:"INFLUX_TOKEN"`
}

func (c Config) Enabled() bool { return strings.TrimSpace(c.URL) != "" }

func (c Config) Validate() error {
	if !c.Enabled() {
		return nil
	}
	if c.Bucket == "" {
		return errs.New(errs.CategoryConfig, "INFLUX_NO_BUCKET",
			"influx: bucket is required when a URL is set")
	}
	if c.Org == "" {
		return errs.New(errs.CategoryConfig, "INFLUX_NO_ORG",
			"influx: org is required when a URL is set")
	}
	return nil
}

type Writer struct {
	cfg    Config
	logger *logrus.Logger
	http   *http.Client
	lines  chan string

	dropped atomic.Uint64
	written atomic.Uint64
}

// New returns nil when no URL is configured, so persistence stays optional and
// the caller can hold a nil *Writer safely.
func New(cfg Config, logger *logrus.Logger) (*Writer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if !cfg.Enabled() {
		return nil, nil
	}
	return &Writer{
		cfg:    cfg,
		logger: logger,
		http:   &http.Client{Timeout: 15 * time.Second},
		lines:  make(chan string, defaultBuffer),
	}, nil
}

func (w *Writer) Name() string { return "influx" }

func (w *Writer) Bucket() string {
	if w == nil {
		return ""
	}
	return w.cfg.Bucket
}

type Stats struct {
	Queued  int    `json:"queued"`
	Written uint64 `json:"written"`
	Dropped uint64 `json:"dropped"`
}

func (w *Writer) Stats() Stats {
	if w == nil {
		return Stats{}
	}
	return Stats{Queued: len(w.lines), Written: w.written.Load(), Dropped: w.dropped.Load()}
}

// Point queues one measurement. A zero timestamp means now.
func (w *Writer) Point(measurement string, tags map[string]string, fields map[string]any, ts time.Time) {
	if w == nil || len(fields) == 0 {
		return
	}
	line := encode(measurement, tags, fields, ts)
	if line == "" {
		return
	}
	select {
	case w.lines <- line:
	default:
		// Persistence must never stall the decision path it observes.
		w.dropped.Add(1)
	}
}

// Start batches queued points until ctx ends, then flushes what is left.
func (w *Writer) Start(ctx context.Context) error {
	if w == nil {
		return nil
	}
	ticker := time.NewTicker(flushEvery)
	defer ticker.Stop()

	w.logger.WithFields(logrus.Fields{
		"url":    w.cfg.URL,
		"org":    w.cfg.Org,
		"bucket": w.cfg.Bucket,
	}).Info("time-series persistence enabled")

	buf := make([]string, 0, batchSize)
	for {
		select {
		case <-ctx.Done():
			w.flush(context.Background(), w.take(buf))
			return nil
		case line := <-w.lines:
			buf = append(buf, line)
			if len(buf) >= batchSize {
				buf = w.take(buf)
			}
		case <-ticker.C:
			buf = w.take(buf)
		}
	}
}

func (w *Writer) take(buf []string) []string {
	if len(buf) == 0 {
		return buf
	}
	w.flush(context.Background(), buf)
	return buf[:0]
}

func (w *Writer) flush(ctx context.Context, lines []string) []string {
	if len(lines) == 0 {
		return lines
	}
	body := strings.Join(lines, "\n")
	url := fmt.Sprintf("%s/api/v2/write?org=%s&bucket=%s&precision=ns",
		strings.TrimRight(w.cfg.URL, "/"), w.cfg.Org, w.cfg.Bucket)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBufferString(body))
	if err != nil {
		return lines
	}
	req.Header.Set("Authorization", "Token "+w.cfg.Token)
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	req.Header.Set("User-Agent", rappsdk.UserAgent)

	resp, err := w.http.Do(req)
	if err != nil {
		w.logger.WithError(err).Warn("time-series write failed")
		return lines
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 300 {
		w.logger.WithField("status", resp.StatusCode).Warn("time-series write rejected")
		return lines
	}
	w.written.Add(uint64(len(lines)))
	return lines
}

// Flush writes everything queued right now, for callers that cannot wait for
// the batch timer.
func (w *Writer) Flush(ctx context.Context) {
	if w == nil {
		return
	}
	var buf []string
	for {
		select {
		case line := <-w.lines:
			buf = append(buf, line)
		default:
			w.flush(ctx, buf)
			return
		}
	}
}

// Query runs Flux and returns the annotated CSV the store replies with.
func (w *Writer) Query(ctx context.Context, flux string) ([]byte, error) {
	if w == nil {
		return nil, errs.New(errs.CategoryInternal, "INFLUX_NOT_CONFIGURED",
			"influx: no store configured")
	}
	url := fmt.Sprintf("%s/api/v2/query?org=%s", strings.TrimRight(w.cfg.URL, "/"), w.cfg.Org)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBufferString(flux))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Token "+w.cfg.Token)
	req.Header.Set("Content-Type", "application/vnd.flux")
	req.Header.Set("Accept", "text/csv")
	req.Header.Set("User-Agent", rappsdk.UserAgent)

	resp, err := w.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, errs.Newf(errs.CategoryPlatform, "INFLUX_QUERY_REJECTED",
			"influx: query returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body))).
			WithStatus(resp.StatusCode)
	}
	return body, nil
}

func encode(measurement string, tags map[string]string, fields map[string]any, ts time.Time) string {
	var b strings.Builder
	b.WriteString(escapeKey(measurement))

	for _, k := range sortedKeys(tags) {
		v := tags[k]
		if v == "" {
			continue
		}
		b.WriteByte(',')
		b.WriteString(escapeKey(k))
		b.WriteByte('=')
		b.WriteString(escapeKey(v))
	}

	first := true
	for _, k := range sortedFieldKeys(fields) {
		encoded, ok := encodeField(fields[k])
		if !ok {
			continue
		}
		if first {
			b.WriteByte(' ')
			first = false
		} else {
			b.WriteByte(',')
		}
		b.WriteString(escapeKey(k))
		b.WriteByte('=')
		b.WriteString(encoded)
	}
	if first {
		return ""
	}

	if ts.IsZero() {
		ts = time.Now()
	}
	b.WriteByte(' ')
	b.WriteString(strconv.FormatInt(ts.UnixNano(), 10))
	return b.String()
}

func encodeField(v any) (string, bool) {
	switch t := v.(type) {
	case float64:
		return strconv.FormatFloat(t, 'g', -1, 64), true
	case float32:
		return strconv.FormatFloat(float64(t), 'g', -1, 32), true
	case int:
		return strconv.Itoa(t) + "i", true
	case int64:
		return strconv.FormatInt(t, 10) + "i", true
	case uint64:
		return strconv.FormatUint(t, 10) + "u", true
	case bool:
		return strconv.FormatBool(t), true
	case string:
		return strconv.Quote(t), true
	default:
		return "", false
	}
}

var keyEscaper = strings.NewReplacer(",", `\,`, " ", `\ `, "=", `\=`)

func escapeKey(s string) string { return keyEscaper.Replace(s) }

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedFieldKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
