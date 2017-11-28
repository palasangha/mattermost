// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package main

import "github.com/spf13/cobra"

var benchmarkCmd = &cobra.Command{
	Use:   "bench",
	Short: "Benchmarks to run",
}

var benchmarkPostingCmd = &cobra.Command{
	Use:   "posting",
	Short: "Benchmark posting of messages",
	RunE:  benchmarkPostingCmdF,
}

func init() {
	benchmarkCmd.AddCommand(benchmarkPostingCmd)
}

func benchmarkPostingCmdF(cmd *cobra.Command, args []string) error {
	return nil
}
