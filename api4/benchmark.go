// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package api4

import (
	"encoding/json"
	"net/http"

	l4g "github.com/alecthomas/log4go"
)

type BenchmarkResult struct {
	Name    string `json:"name"`
	NsPerOp int64  `json:"ns_per_op"`
	Error   string `json:"error"`
}

type BenchmarkResults struct {
	Results []BenchmarkResult `json:"results"`
}

func (b *BenchmarkResults) ToJson() string {
	bytes, err := json.Marshal(b)
	if err != nil {
		return ""
	} else {
		return string(bytes)
	}
}

func (api *API) InitBenchmark() {
	l4g.Debug("Initalizing Benchmark API Routes")

	api.BaseRoutes.Benchmark.Handle("/posting", api.ApiHandler(runPostingBenchmark))
}

func runPostingBenchmark(c *Context, w http.ResponseWriter, r *http.Request) {
	results := c.App.RunBenchmarks()

	output := BenchmarkResults{}
	for _, result := range results {
		err := ""
		if result.Error != nil {
			err = result.Error.Error()
		}
		output.Results = append(output.Results, BenchmarkResult{
			Name:    result.Name,
			NsPerOp: result.TimePerOp.Nanoseconds(),
			Error:   err,
		})
	}
	w.Write([]byte(output.ToJson()))
}
