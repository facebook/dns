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
	"strings"

	"github.com/facebook/dns/dnswatch/snoop"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(snoopCmd)
	snoopCmd.Flags().StringVar(&cfg.Fields, "list", "PID,PNAME,TYPE,QNAME,RCODE", "fields displayed in stdout \n"+
		fmt.Sprintf("all fields: %s", strings.Join(snoop.AllFieldNames(), " ")))
	snoopCmd.Flags().BoolVar(&cfg.FilterDebug, "filterdebug", false, "debug only filter using stdout")
	snoopCmd.Flags().BoolVar(&cfg.ProbeDebug, "probedebug", false, "debug only probe using stdout")
}

var snoopCmd = &cobra.Command{
	Use:   "snoop",
	Short: "Display all DNS activity on the host in real-time",
	Long: `Display all DNS activity on the host in real-time

Usage example:
  dnswatch snoop --list PID,PNAME,QNAME,RIP
`,
	Run: func(cmd *cobra.Command, args []string) {
		ConfigureVerbosity()

		if err := snoop.Run(&cfg); err != nil {
			log.Fatalf("unable to run snoop: %v", err)
		}
	},
}
