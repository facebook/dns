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
	RootCmd.AddCommand(detailedCmd)
}

var detailedCmd = &cobra.Command{
	Use:   "detailed",
	Short: "Display all DNS activity on the host. Prints details to stdout in detailed dig-like format",
	Long: `Display all DNS activity on the host. Prints details to stdout in detailed dig-like format

Usage example:
  dnswatch detailed
`,

	Run: func(cmd *cobra.Command, args []string) {
		ConfigureVerbosity()

		cfg.Detailed = true
		if err := snoop.Run(&cfg); err != nil {
			log.Fatalf("unable to run snoop: %v", err)
		}
	},
}
