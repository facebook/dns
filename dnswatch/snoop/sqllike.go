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

package snoop

import (
	"os"
	"strings"

	df "github.com/go-gota/gota/dataframe"
	"github.com/go-gota/gota/series"
	log "github.com/sirupsen/logrus"
)

// SqllikeData used to store filtering information
// and the dataframe table
type SqllikeData struct {
	Where   string
	Orderby string
	Groupby string
	Df      df.DataFrame
}

// Setup used to create dataframe from list of maps
func (s *SqllikeData) Setup(m []map[string]interface{}) {
	s.Df = df.LoadMaps(m)
}

// SolveOrderby sorts data based on the Orderby priority list
// ex: QNAME;-LATENCY means first sort by qname then reverse sort by latency
func (s *SqllikeData) SolveOrderby() {
	cols := strings.Split(s.Orderby, ",")
	for i := len(cols) - 1; i >= 0; i-- {
		col := cols[i]
		if len(col) == 0 {
			continue
		}
		if col[0] == '-' {
			s.Df = s.Df.Arrange(df.RevSort(col[1:]))
		} else {
			s.Df = s.Df.Arrange(df.Sort(col))
		}
	}
}

// SolveWhere filters the data based on the Where list
// ex: PNAME=smcc;LATENCY>200 means display only rows with
// PNAME = smcc and latency > 200 microseconds
func (s *SqllikeData) SolveWhere() {
	conditions := strings.Split(s.Where, ",")
	for _, cond := range conditions {
		if len(cond) == 0 {
			continue
		}
		var comparator series.Comparator
		var condSplit []string
		if condSplit = strings.Split(cond, "="); len(condSplit) > 1 {
			comparator = series.Eq
		} else if condSplit = strings.Split(cond, ">"); len(condSplit) > 1 {
			comparator = series.Greater
		} else if condSplit = strings.Split(cond, "<"); len(condSplit) > 1 {
			comparator = series.Less
		} else {
			log.Error("only =,<,> accepted as comparators in where list")
			continue
		}
		s.Df = s.Df.Filter(
			df.F{
				Colname:    condSplit[0],
				Comparator: comparator,
				Comparando: condSplit[1],
			},
		)
	}
}

// SolveGroupby groups columns based on the groupby list
func (s *SqllikeData) SolveGroupby() {
	if s.Groupby == "" {
		return
	}
	var groupCols []string
	aux := strings.Split(s.Groupby, ",")
	for _, str := range aux {
		if str != "" {
			groupCols = append(groupCols, str)
		}
	}
	groups := s.Df.GroupBy(groupCols...)

	var aggArray []df.AggregationType
	var colArray []string

	aggArray = append(aggArray, df.Aggregation_COUNT)
	colArray = append(colArray, "LATENCY")

	aggArray = append(aggArray, df.Aggregation_MAX)
	colArray = append(colArray, "LATENCY")

	aggArray = append(aggArray, df.Aggregation_MIN)
	colArray = append(colArray, "LATENCY")

	aggArray = append(aggArray, df.Aggregation_MEAN)
	colArray = append(colArray, "LATENCY")

	aggArray = append(aggArray, df.Aggregation_MEDIAN)
	colArray = append(colArray, "LATENCY")

	s.Df = groups.Aggregation(aggArray, colArray)

	s.Df = s.Df.Arrange(df.RevSort("LATENCY_MEAN"))
}

// Print used to display on stdout the dataframe
func (s *SqllikeData) Print(path string) {
	f, err := os.Create(path)
	if err != nil {
		log.Errorf("unable to create csv: %v\n", err)
		return
	}
	defer f.Close()

	if err := s.Df.WriteCSV(f); err != nil {
		log.Errorf("unable to write to file: %v\n", err)
	}
}
