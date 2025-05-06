// Copyright 2025 Jan-Niclas Struewer. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package beehive

import (
	"sync"
)

// The BufferedCollector stores elements in an internal
// buffer. Once the BufferSize is reached it uses the
// Collect method to process all buffered elements.
// It implements the Fan-In process of the Fan-Out and
// Fan-In pattern.
type BufferedCollector[T any] struct {
	Collect    func(t []*T) error // processes collected elements
	BufferSize int                // size of the internal buffer
}

type BufferedCollectorConfig struct {
	BufferSize *int // optional, defaults to 1, meaning no buffering
}

// Initialize a new BufferedCollector with the given configuration
// and default values for all optional fields.
func NewBufferedCollector[T any](
	collect func(t []*T) error,
	config BufferedCollectorConfig,
) *BufferedCollector[T] {

	buffer := 1
	if config.BufferSize != nil && *config.BufferSize > 1 {
		buffer = *config.BufferSize
	}

	return &BufferedCollector[T]{
		BufferSize: buffer,
		Collect:    collect,
	}
}

// Starts the collection process. The collector runs as long as in is open.
// Once in is closed it calls collect on the remaining elements in the buffer.
func (w *BufferedCollector[T]) Run(in <-chan *T, errc chan error, wg *sync.WaitGroup) {
	defer wg.Done()

	buffer := make([]*T, 0, w.BufferSize)

	for i := range in {
		if len(buffer) >= w.BufferSize {
			err := w.Collect(buffer)
			if err != nil {
				errc <- err
			}
			// Reset length to 0, keeping the allocated capacity
			buffer = buffer[:0]
		}

		buffer = append(buffer, i)
	}

	// empty buffer once in is closed
	if len(buffer) > 0 {
		err := w.Collect(buffer)
		if err != nil {
			errc <- err
		}
	}
}
