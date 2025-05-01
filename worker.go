// Copyright 2025 Jan-Niclas Struewer. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package beehive

import "sync"

// The Worker processes the elements that are provided
// to it by the Fan-Out step of the dispatcher.
type Worker[T, E any] struct {
	Do func(*T) (*E, error) // function to be executed by the worker
}

// starts the worker. It accepts inputs until the in channel is closed.
// wg communicates to the dispatcher that the worker has finished.
func (w *Worker[T, E]) Run(in <-chan *T, out chan *E, errc chan error, wg *sync.WaitGroup) {
	defer wg.Done()
	for i := range in {
		res, err := w.Do(i)
		if err != nil {
			errc <- err
			continue
		}
		out <- res
	}
}

// helper function for workers that pass their
// input to the output without modification.
// This is useful to use the workerpool to read
// data and store it to another source (file to database)
func DoNothing[T any](t *T) (*T, error) {
	return t, nil
}
