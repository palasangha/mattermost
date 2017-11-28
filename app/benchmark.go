// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package app

import (
	"fmt"
	"math/rand"
	"time"

	l4g "github.com/alecthomas/log4go"
	"github.com/mattermost/mattermost-server/model"
)

const (
	BENCHMARK_POST_MESSAGE    = "This is a benchmarking post."
	BENCHMARK_FAKE_CHANNEL_ID = "channelaaaaaaaaaaaaaaaaaab"
	BENCHMARK_FAKE_USER_ID    = "useraaaaaaaaaaaaaaaaaaaaab"
	BENCHMARK_ITERATIONS      = 1000
)

type Benchmark interface {
	Name() string
	Init(interations int) *model.AppError
	Task(i int) *model.AppError
	Cleanup(iterations int) *model.AppError
}

type BenchmarkResult struct {
	Name      string
	TimePerOp time.Duration
	Error     *model.AppError
}

func (a *App) GetBenchmarks() []Benchmark {
	return []Benchmark{
		&BenchmarkPosting{a},
		&BenchmarkReadSinglePost{a: a},
		&BenchmarkReadChannel{a: a, numPosts: 700, numReplies: 300},
		&BenchmarkGetChannelEtag{a: a, numPosts: 700, numReplies: 300},
	}
}

func (a *App) RunBenchmarks() []BenchmarkResult {
	benchmarks := a.GetBenchmarks()
	results := make([]BenchmarkResult, 0, len(benchmarks))

	for _, benchmark := range benchmarks {
		l4g.Debug("Init %v Benchmark", benchmark.Name())
		benchmark.Init(BENCHMARK_ITERATIONS)
		l4g.Debug("Performing %v Benchmark", benchmark.Name())
		timePerOp, err := doBenchmark(BENCHMARK_ITERATIONS, benchmark)
		l4g.Debug("Cleanup %v Benchmark", benchmark.Name())
		benchmark.Cleanup(BENCHMARK_ITERATIONS)
		if err != nil {
			l4g.Debug("%v Benchmark encountered error: %v", benchmark.Name(), err.Error())
		} else {
			l4g.Debug("%v Benchmark Time: %vns/op", benchmark.Name(), timePerOp.Nanoseconds())
		}
		results = append(results, BenchmarkResult{
			Name:      benchmark.Name(),
			TimePerOp: timePerOp,
			Error:     err,
		})
	}

	return results
}

func doBenchmark(iterations int, benchmark Benchmark) (time.Duration, *model.AppError) {
	begin := time.Now()
	for i := 0; i < iterations; i++ {
		if err := benchmark.Task(i); err != nil {
			return 0, err
		}
	}
	end := time.Now()
	elapsed := end.Sub(begin)

	return elapsed / time.Duration(iterations), nil
}

func getBenchmarkPost() *model.Post {
	return &model.Post{
		Message:   BENCHMARK_POST_MESSAGE,
		ChannelId: BENCHMARK_FAKE_CHANNEL_ID,
		UserId:    BENCHMARK_FAKE_USER_ID,
	}
}

func getBenchmarkReplyTo(parent string) *model.Post {
	return &model.Post{
		Message:   BENCHMARK_POST_MESSAGE,
		ChannelId: BENCHMARK_FAKE_CHANNEL_ID,
		UserId:    BENCHMARK_FAKE_USER_ID,
		ParentId:  parent,
		RootId:    parent,
	}
}

func (a *App) deleteBenchmarkPosts() *model.AppError {
	if result := <-a.Srv.Store.Post().PermanentDeleteByChannel(BENCHMARK_FAKE_CHANNEL_ID); result.Err != nil {
		return result.Err
	}
	return nil
}

///
/// Posting Benchmark
///
type BenchmarkPosting struct {
	a *App
}

func (b *BenchmarkPosting) Name() string                        { return "Posting" }
func (b *BenchmarkPosting) Init(iterations int) *model.AppError { return nil }
func (b *BenchmarkPosting) Task(i int) *model.AppError {
	post := getBenchmarkPost()
	if result := <-b.a.Srv.Store.Post().Save(post); result.Err != nil {
		return result.Err
	}
	return nil
}
func (b *BenchmarkPosting) Cleanup(iterations int) *model.AppError {
	return b.a.deleteBenchmarkPosts()
}

///
/// Read Single Post Benchmark
///
type BenchmarkReadSinglePost struct {
	a       *App
	postIds []string
}

func (b *BenchmarkReadSinglePost) Name() string { return "Read Single Post" }
func (b *BenchmarkReadSinglePost) Init(iterations int) *model.AppError {
	b.postIds = make([]string, 0, iterations)
	for i := 0; i < iterations; i++ {
		post := getBenchmarkPost()
		if result := <-b.a.Srv.Store.Post().Save(post); result.Err != nil {
			return result.Err
		} else {
			b.postIds = append(b.postIds, result.Data.(*model.Post).Id)
		}
	}
	return nil
}
func (b *BenchmarkReadSinglePost) Task(i int) *model.AppError {
	if result := <-b.a.Srv.Store.Post().GetSingle(b.postIds[i]); result.Err != nil {
		return result.Err
	}
	return nil
}
func (b *BenchmarkReadSinglePost) Cleanup(iterations int) *model.AppError {
	return b.a.deleteBenchmarkPosts()
}

///
/// Read Channel Benchmark
///
type BenchmarkReadChannel struct {
	a          *App
	numPosts   int
	numReplies int
}

func (b *BenchmarkReadChannel) Name() string {
	return fmt.Sprintf("Read Channel at %v posts %v replies", b.numPosts, b.numReplies)
}
func createPostsAndReplies(a *App, numPosts, numReplies int) *model.AppError {
	postIds := make([]string, 0, numPosts+numReplies)
	for i := 0; i < numPosts; i++ {
		post := getBenchmarkPost()
		if result := <-a.Srv.Store.Post().Save(post); result.Err != nil {
			return result.Err
		} else {
			postIds = append(postIds, result.Data.(*model.Post).Id)
		}
	}
	for i := 0; i < numPosts; i++ {
		reply := getBenchmarkReplyTo(postIds[rand.Intn(len(postIds))])
		if result := <-a.Srv.Store.Post().Save(reply); result.Err != nil {
			return result.Err
		} else {
			postIds = append(postIds, result.Data.(*model.Post).Id)
		}
	}
	return nil
}
func (b *BenchmarkReadChannel) Init(iterations int) *model.AppError {
	return createPostsAndReplies(b.a, b.numPosts, b.numReplies)
}
func (b *BenchmarkReadChannel) Task(i int) *model.AppError {
	if result := <-b.a.Srv.Store.Post().GetPosts(BENCHMARK_FAKE_CHANNEL_ID, 0, 60, false); result.Err != nil {
		return result.Err
	}
	return nil
}
func (b *BenchmarkReadChannel) Cleanup(iterations int) *model.AppError {
	return b.a.deleteBenchmarkPosts()
}

///
/// Get Channel Etag Benchmark
///
type BenchmarkGetChannelEtag struct {
	a          *App
	numPosts   int
	numReplies int
}

func (b *BenchmarkGetChannelEtag) Name() string {
	return fmt.Sprintf("Get Channel Etag at %v posts %v replies", b.numPosts, b.numReplies)
}
func (b *BenchmarkGetChannelEtag) Init(interations int) *model.AppError {
	return createPostsAndReplies(b.a, b.numPosts, b.numReplies)
}
func (b *BenchmarkGetChannelEtag) Task(i int) *model.AppError {
	if result := <-b.a.Srv.Store.Post().GetEtag(BENCHMARK_FAKE_CHANNEL_ID, false); result.Err != nil {
		return result.Err
	}
	return nil
}
func (b *BenchmarkGetChannelEtag) Cleanup(iterations int) *model.AppError {
	if result := <-b.a.Srv.Store.Post().GetPosts(BENCHMARK_FAKE_CHANNEL_ID, 0, 60, false); result.Err != nil {
		return result.Err
	}
	return nil
}
