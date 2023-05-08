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
	"github.com/facebookincubator/dns/dnswatch/snoop"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(sqlCmd)
	sqlCmd.Flags().StringVar(&cfg.Csv, "csv", "", "csv output file")
	sqlCmd.Flags().StringVar(&cfg.Where, "where", "", "sql where statement")
	sqlCmd.Flags().StringVar(&cfg.Orderby, "orderby", "", "sql orderby statement")
	sqlCmd.Flags().StringVar(&cfg.Groupby, "groupby", "", "sql groupby statement")
}

var sqlCmd = &cobra.Command{
	Use:   "sql",
	Short: "Transform and display DNS activity using 'where', 'orderby', 'groupby'; Supports printing data to csv",
	Long: `Transform and display DNS activity using 'where', 'orderby', 'groupby'; Supports printing data to csv

Usage example:
  dnswatch sql --csv /tmp/dnswatch_out --period 30s --orderby -LATENCY --groupby PNAME,QNAME
`,

	Run: func(cmd *cobra.Command, args []string) {
		ConfigureVerbosity()

		if cfg.Csv == "" {
			log.Fatal("no output file")
		}

		cfg.Sqllike = true
		if err := snoop.Run(&cfg); err != nil {
			log.Fatalf("unable to run snoop: %v", err)
		}
	},
}
