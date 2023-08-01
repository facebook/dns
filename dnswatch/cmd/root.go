/*
Copyright (c) Facebook, Inc. and its affiliates.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/facebook/dns/dnswatch/snoop"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// RootCmd is a main entry point. It's exported so dnswatch could be easily extended without touching core functionality.
var RootCmd = &cobra.Command{
	Use:   "dnswatch",
	Short: "Monitor for DNS traffic",
}

var cfg snoop.Config

func init() {
	RootCmd.PersistentFlags().StringVar(&cfg.LogLevel, "loglevel", "info", "set a log level. Can be: trace, debug, info, warning, error")
	RootCmd.PersistentFlags().StringVar(&cfg.Interface, "if", "", "network interface to use")
	RootCmd.PersistentFlags().IntVar(&cfg.Port, "port", 53, "port number")
	RootCmd.PersistentFlags().IntVar(&cfg.RingSizeMB, "ringsize", 10, "ring size (MB) used to store packets")
	RootCmd.PersistentFlags().DurationVar(&cfg.CleanPeriod, "period", 3*time.Second, "monitoring timeframe before writing to csv or refreshing the screen")
}

// ConfigureVerbosity configures log verbosity based on parsed flags. Needs to be called by any subcommand.
func ConfigureVerbosity() {
	switch cfg.LogLevel {
	case "trace":
		log.SetLevel(log.TraceLevel)
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "warning":
		log.SetLevel(log.WarnLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	default:
		log.Fatalf("Unrecognized log level: %v", cfg.LogLevel)
	}
}

// Execute is the main entry point for CLI interface
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
