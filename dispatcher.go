// Copyright 2025 Jan-Niclas Struewer. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package beehive

import (
	"io"
	"iter"
	"log/slog"
	"math"
	"runtime"
	"sync"
	"time"
)

// The Dispatcher implements the Fan-Out and Fan-In pattern.
// It manages a pool of workers to process a sequence of tasks.
// It is responsible for distributing tasks to workers (Fan-Out) and
// collecting results (Fan-In).
type Dispatcher[T, E any] struct {
	NumWorker int
	Worker    Worker[T, E]
	Producer  iter.Seq[T]
	Collector BufferedCollector[E]

	logger        *slog.Logger
	channelBuffer int
	rateLimit     time.Duration
}

type DispatcherConfig struct {
	NumWorker *int           // optional, defaults to runtime.NumCPU()
	RateLimit *time.Duration // optional, defaults to 0 meaning no rate limit is applied
	Logger    *slog.Logger   // optional, defaults to slog.New(slog.NewTextHandler(io.Discard, nil))
}

// NewDispatcher creates a new Dispatcher instance based on the
// provided configuration. It provides sensible defaults for
// all optional parameters according to the description
// in the DispatcherConfig.
func NewDispatcher[T, E any](Worker Worker[T, E],
	Producer iter.Seq[T],
	ResultCollector BufferedCollector[E],
	config DispatcherConfig) *Dispatcher[T, E] {

	if config.Logger == nil {
		handler := slog.NewTextHandler(io.Discard, nil)
		config.Logger = slog.New(handler)
	}

	d := Dispatcher[T, E]{
		Worker:    Worker,
		Producer:  Producer,
		Collector: ResultCollector,
		logger:    config.Logger,
	}

	// set number of workers
	var numWorker int
	if config.NumWorker != nil {
		if *config.NumWorker <= 0 {
			numWorker = runtime.NumCPU()
		} else {
			numWorker = *config.NumWorker
		}
	} else {
		numWorker = runtime.NumCPU()
	}
	d.NumWorker = numWorker

	if config.RateLimit != nil && *config.RateLimit > 0 {
		d.rateLimit = *config.RateLimit
	} else {
		d.rateLimit = 0
	}

	// calculate channel buffer size
	channelBuffer := d.Collector.BufferSize
	if channelBuffer == math.MaxInt {
		channelBuffer = 100 // limit channel buffer size
	}
	d.channelBuffer = channelBuffer / numWorker

	return &d
}

// Starts the dispatching process. Spawns the configured number of
// workers and collector. The workers and collector are connected
// via channels. The function waits for all workers and collector
// to finish before returning. Workers are done once all elements
// from the producer have been processed. Thus, terminating the
// sequence ends the dispatching.
//
//	This is a blocking function!
func (d *Dispatcher[T, E]) Dispatch() *[]error {

	in := make(chan *T)
	out := make(chan *E, d.channelBuffer)
	errc := make(chan error)

	// used to signal when worker and collector are done
	var processWg sync.WaitGroup
	var collectWg sync.WaitGroup
	var errorWg sync.WaitGroup

	// setup workers
	for range d.NumWorker {
		processWg.Add(1)
		go d.Worker.Run(in, out, errc, &processWg)
	}
	d.logger.Debug("Started workers", "number workers", d.NumWorker)

	errs := []error{}
	// setup error logging
	errorWg.Add(1)
	go func() {
		defer errorWg.Done()
		for err := range errc {
			d.logger.Error("an error occurred in dispatcher", "error", err)
			errs = append(errs, err)
		}
	}()

	// setup writer
	collectWg.Add(1)
	go d.Collector.Run(out, errc, &collectWg)

	// start producer
	if d.rateLimit > 0 {
		throttle := time.Tick(d.rateLimit)
		for e := range d.Producer {
			// blocks until we recive a new tick
			<-throttle
			in <- &e
		}
	} else {
		for e := range d.Producer {
			in <- &e
		}
	}

	// order matters !
	// we close in when there is no more element in the producer
	close(in)
	d.logger.Debug("Producer finished")

	// we close out when all workers are finished
	processWg.Wait()
	close(out)
	d.logger.Debug("Worker finished")

	// we close errc and terminate after the collector is finished
	collectWg.Wait()
	close(errc)
	d.logger.Debug("Collector finished")

	errorWg.Wait()

	if len(errs) == 0 {
		return nil
	}

	return &errs
}
