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

// Package kafkasrc reads a topic exposed by the platform's message bus, the
// transport a DME job uses when it delivers over Kafka rather than HTTP.
package kafkasrc

import (
	"context"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl"
	"github.com/segmentio/kafka-go/sasl/scram"
	"github.com/sirupsen/logrus"

	"github.com/coranlabs/OTAF/otaf-rapp-sdk/errs"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/ingest"
)

type Config struct {
	Brokers  []string `yaml:"brokers" env:"KAFKA_BROKERS"`
	Topic    string   `yaml:"topic" env:"KAFKA_TOPIC"`
	Group    string   `yaml:"group" env:"KAFKA_CONSUMER_GROUP"`
	Username string   `yaml:"username" env:"KAFKA_USERNAME"`
	Password string   `yaml:"password" env:"KAFKA_PASSWORD"`
}

func (c Config) Validate() error {
	if len(c.Brokers) == 0 {
		return errs.New(errs.CategoryConfig, "KAFKA_NO_BROKERS",
			"kafka: no brokers configured")
	}
	if c.Topic == "" {
		return errs.New(errs.CategoryConfig, "KAFKA_NO_TOPIC",
			"kafka: no topic configured")
	}
	return nil
}

type Source struct {
	cfg    Config
	reader *kafka.Reader
	logger *logrus.Logger
}

func New(cfg Config, logger *logrus.Logger) (*Source, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if cfg.Group == "" {
		cfg.Group = "rapp-consumer"
	}

	dialer := &kafka.Dialer{Timeout: 30 * time.Second, DualStack: true}
	if cfg.Username != "" && cfg.Password != "" {
		var mechanism sasl.Mechanism
		mechanism, err := scram.Mechanism(scram.SHA512, cfg.Username, cfg.Password)
		if err != nil {
			return nil, errs.Wrap(err, errs.CategoryConfig, "KAFKA_BAD_CREDENTIALS",
				"kafka: could not set up SCRAM-SHA-512")
		}
		dialer.SASLMechanism = mechanism
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        cfg.Brokers,
		Topic:          cfg.Topic,
		GroupID:        cfg.Group,
		Dialer:         dialer,
		MinBytes:       1,
		MaxBytes:       10 << 20,
		MaxWait:        10 * time.Second,
		CommitInterval: 5 * time.Second,
		StartOffset:    kafka.LastOffset,
		Logger:         kafka.LoggerFunc(func(m string, a ...interface{}) { logger.Debugf(m, a...) }),
		ErrorLogger:    kafka.LoggerFunc(func(m string, a ...interface{}) { logger.Errorf(m, a...) }),
	})

	return &Source{cfg: cfg, reader: reader, logger: logger}, nil
}
