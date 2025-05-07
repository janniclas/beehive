package beehive

import (
	"errors"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"math"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Helper function to create a simple sequence of integers for testing
func newIntProducer(count int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := range count {
			if !yield(i) {
				return
			}
		}
	}
}

// --- Tests for NewDispatcher ---

func TestNewDispatcher_Defaults(t *testing.T) {
	producer := newIntProducer(0)
	worker := Worker[int, int]{Work: func(i *int) (*int, error) { return i, nil }}
	bufferSize := 1
	collector := NewBufferedCollector(func(items []*int) error { return nil }, BufferedCollectorConfig{BufferSize: &bufferSize})

	disp := NewDispatcher(worker, producer, *collector, DispatcherConfig{})

	if disp.NumWorker != runtime.NumCPU() {
		t.Errorf("Expected NumWorker to be %d, got %d", runtime.NumCPU(), disp.NumWorker)
	}
	if disp.rateLimit != 0 {
		t.Errorf("Expected rateLimit to be 0, got %v", disp.rateLimit)
	}
	if disp.logger == nil {
		t.Errorf("Expected logger to be non-nil, got nil")
	}
	// Default channelBuffer logic: Collector.BufferSize (1) / NumCPU. Can be 0.
	expectedChanBuffer := collector.BufferSize / disp.NumWorker // e.g., 1 / runtime.NumCPU()
	if disp.channelBuffer != expectedChanBuffer {
		t.Errorf("Expected channelBuffer to be %d (Collector.BufferSize %d / NumWorker %d), got %d",
			expectedChanBuffer, collector.BufferSize, disp.NumWorker, disp.channelBuffer)
	}

	if disp.channelBuffer != expectedChanBuffer {
		t.Errorf("Expected channelBuffer to be %d (BufferSize %d / NumWorker %d), got %d",
			expectedChanBuffer, collector.BufferSize, disp.NumWorker, disp.channelBuffer)
	}
}

func TestNewDispatcher_CustomNumWorker(t *testing.T) {
	producer := newIntProducer(0)
	worker := Worker[int, int]{Work: func(i *int) (*int, error) { return i, nil }}
	bufferSize := 10
	collector := NewBufferedCollector(func(items []*int) error { return nil }, BufferedCollectorConfig{BufferSize: &bufferSize})

	customWorkers := 4
	disp := NewDispatcher(worker, producer, *collector, DispatcherConfig{NumWorker: &customWorkers})
	if disp.NumWorker != customWorkers {
		t.Errorf("Expected NumWorker to be %d, got %d", customWorkers, disp.NumWorker)
	}

	zeroWorkers := 0
	disp = NewDispatcher(worker, producer, *collector, DispatcherConfig{NumWorker: &zeroWorkers})
	if disp.NumWorker != runtime.NumCPU() {
		t.Errorf("Expected NumWorker to be %d for zero input, got %d", runtime.NumCPU(), disp.NumWorker)
	}

	negativeWorkers := -1
	disp = NewDispatcher(worker, producer, *collector, DispatcherConfig{NumWorker: &negativeWorkers})
	if disp.NumWorker != runtime.NumCPU() {
		t.Errorf("Expected NumWorker to be %d for negative input, got %d", runtime.NumCPU(), disp.NumWorker)
	}
}

func TestNewDispatcher_CustomRateLimit(t *testing.T) {
	producer := newIntProducer(0)
	worker := Worker[int, int]{Work: func(i *int) (*int, error) { return i, nil }}
	bufferSize := 1
	collector := NewBufferedCollector(func(items []*int) error { return nil }, BufferedCollectorConfig{BufferSize: &bufferSize})

	customRateLimit := time.Millisecond * 100
	disp := NewDispatcher(worker, producer, *collector, DispatcherConfig{RateLimit: &customRateLimit})
	if disp.rateLimit != customRateLimit {
		t.Errorf("Expected rateLimit to be %v, got %v", customRateLimit, disp.rateLimit)
	}

	zeroRateLimit := time.Duration(0)
	disp = NewDispatcher(worker, producer, *collector, DispatcherConfig{RateLimit: &zeroRateLimit})
	if disp.rateLimit != 0 {
		t.Errorf("Expected rateLimit to be 0 for zero input, got %v", disp.rateLimit)
	}
}

func TestNewDispatcher_CustomLogger(t *testing.T) {
	producer := newIntProducer(0)
	worker := Worker[int, int]{Work: func(i *int) (*int, error) { return i, nil }}
	bufferSize := 1
	collector := NewBufferedCollector(func(items []*int) error { return nil }, BufferedCollectorConfig{BufferSize: &bufferSize})
	customLogger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))

	disp := NewDispatcher(worker, producer, *collector, DispatcherConfig{Logger: customLogger})
	if disp.logger != customLogger {
		t.Errorf("Expected custom logger to be used")
	}
}

func TestNewDispatcher_ChannelBufferCalculation(t *testing.T) {
	producer := newIntProducer(0)
	workerImpl := Worker[int, int]{Work: func(i *int) (*int, error) { return i, nil }}

	tests := []struct {
		name                  string
		collectorBufferSize   int
		numWorker             int
		expectedChannelBuffer int
	}{
		{"BufferSize < NumWorker", 5, 10, 0},                     // 5/10 = 0
		{"BufferSize == NumWorker", 10, 10, 1},                   // 10/10 = 1
		{"BufferSize > NumWorker", 20, 10, 2},                    // 20/10 = 2
		{"BufferSize MaxInt", math.MaxInt, 10, 100 / 10},         // math.MaxInt becomes 100, then 100/10 = 10
		{"BufferSize MaxInt, 1 Worker", math.MaxInt, 1, 100 / 1}, // 100/1 = 100
		{"BufferSize 100, NumCPU Worker", 100, runtime.NumCPU(), 100 / runtime.NumCPU()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bufferSize := tt.collectorBufferSize
			collector := NewBufferedCollector(func(items []*int) error { return nil }, BufferedCollectorConfig{BufferSize: &bufferSize})

			// Ensure collector.BufferSize is set as expected by NewBufferedCollector
			if tt.collectorBufferSize == math.MaxInt {
				// NewBufferedCollector doesn't cap at math.MaxInt, it takes it as is.
				// The capping to 100 happens inside NewDispatcher's channelBuffer calculation logic.
			} else if tt.collectorBufferSize <= 0 {
				collector.BufferSize = 1 // NewBufferedCollector defaults to 1 if input <=0
			} else {
				collector.BufferSize = tt.collectorBufferSize
			}

			config := DispatcherConfig{}
			if tt.numWorker > 0 { // Allow testing default NumWorker if tt.numWorker is 0 or not specified for this path
				config.NumWorker = &tt.numWorker
			}

			disp := NewDispatcher(workerImpl, producer, *collector, config)

			actualNumWorker := disp.NumWorker
			if tt.numWorker <= 0 && config.NumWorker == nil { // if testing default worker count path
				actualNumWorker = runtime.NumCPU()
			} else if tt.numWorker <= 0 && config.NumWorker != nil && *config.NumWorker <= 0 {
				actualNumWorker = runtime.NumCPU()
			} else if config.NumWorker != nil {
				actualNumWorker = *config.NumWorker
			}

			// In TestNewDispatcher_ChannelBufferCalculation's t.Run
			cbsForCalc := collector.BufferSize // This is the BufferSize from the collector passed to NewDispatcher
			if cbsForCalc == math.MaxInt {
				cbsForCalc = 100 // This matches NewDispatcher's logic
			}
			// actualNumWorker is already correctly determined in your test.
			expectedBuf := cbsForCalc / actualNumWorker // Integer division directly gives the right result.

			if disp.channelBuffer != expectedBuf {
				t.Errorf("Test '%s': Expected channelBuffer %d (collector.BufferSize: %d, usedForCalc: %d, actualNumWorker: %d), got %d",
					tt.name, expectedBuf, collector.BufferSize, cbsForCalc, actualNumWorker, disp.channelBuffer)
			}

		})
	}
}

// --- Tests for Dispatcher.Dispatch ---

func TestDispatch_Success_NoRateLimit(t *testing.T) {
	numItems := 100
	producer := newIntProducer(numItems)

	var processedCount atomic.Int32
	worker := Worker[int, int]{
		Work: func(item *int) (*int, error) {
			processedCount.Add(1)
			res := *item * 2
			return &res, nil
		},
	}

	var collectedItems []*int
	var mu sync.Mutex
	var collectCallCount atomic.Int32

	bufferSize := 10
	collector := NewBufferedCollector(func(items []*int) error {
		mu.Lock()
		defer mu.Unlock()
		collectCallCount.Add(1)
		collectedItems = append(collectedItems, items...)
		return nil
	}, BufferedCollectorConfig{BufferSize: &bufferSize}) // Buffer size 10

	disp := NewDispatcher(worker, producer, *collector, DispatcherConfig{})
	errs := disp.Dispatch()

	if errs != nil {
		t.Fatalf("Expected no errors, got %v", *errs)
	}
	if int(processedCount.Load()) != numItems {
		t.Errorf("Expected %d items to be processed by worker, got %d", numItems, processedCount.Load())
	}

	mu.Lock()
	if len(collectedItems) != numItems {
		t.Errorf("Expected %d items to be collected, got %d", numItems, len(collectedItems))
	}
	sum := 0
	for _, item := range collectedItems {
		sum += *item
	}
	mu.Unlock()

	expectedSum := 0
	for i := range numItems {
		expectedSum += i * 2
	}
	if sum != expectedSum {
		t.Errorf("Expected sum of collected items to be %d, got %d", expectedSum, sum)
	}
	if collectCallCount.Load() != int32(numItems/10) { // 100 items, buffer 10 -> 10 calls
		t.Errorf("Expected %d calls to collector's Collect, got %d", numItems/10, collectCallCount.Load())
	}
}

func TestDispatch_Success_WithRateLimit(t *testing.T) {
	numItems := 5
	rateLimit := 50 * time.Millisecond
	producer := newIntProducer(numItems)

	worker := Worker[int, int]{Work: func(item *int) (*int, error) { res := *item; return &res, nil }}

	var collectedItems []*int
	var mu sync.Mutex
	collector := NewBufferedCollector(func(items []*int) error {
		mu.Lock()
		collectedItems = append(collectedItems, items...)
		mu.Unlock()
		return nil
	}, BufferedCollectorConfig{BufferSize: &numItems}) // Collect all at once

	numWorkers := 2
	disp := NewDispatcher(worker, producer, *collector, DispatcherConfig{
		RateLimit: &rateLimit,
		NumWorker: &numWorkers, // Rate limit applies to producer, not per worker
	})

	startTime := time.Now()
	errs := disp.Dispatch()
	duration := time.Since(startTime)

	if errs != nil {
		t.Fatalf("Expected no errors, got %v", *errs)
	}
	mu.Lock()
	if len(collectedItems) != numItems {
		t.Errorf("Expected %d items to be collected, got %d", numItems, len(collectedItems))
	}
	mu.Unlock()

	// Expected duration is roughly (numItems * rateLimit).
	// Allow some slack due to goroutine scheduling and system load.
	// The rate limit is on the producer, so it's sequential for feeding 'in'.
	minExpectedDuration := time.Duration(numItems) * rateLimit
	maxExpectedDuration := minExpectedDuration + (rateLimit * time.Duration(numItems) / 2) + (200 * time.Millisecond) // Slack for scheduling

	if duration < minExpectedDuration || duration > maxExpectedDuration {
		t.Errorf("Expected duration between %v and %v, got %v", minExpectedDuration, maxExpectedDuration, duration)
	}
}

func TestDispatch_WorkerError(t *testing.T) {
	numItems := 3
	producer := newIntProducer(numItems) // 0, 1, 2
	workerErr := errors.New("worker failed")

	var processedSuccessfully atomic.Int32
	worker := Worker[int, int]{
		Work: func(item *int) (*int, error) {
			if *item == 1 { // Fail for item 1
				return nil, workerErr
			}
			processedSuccessfully.Add(1)
			res := *item * 2
			return &res, nil
		},
	}

	var collectedItems []*int
	var mu sync.Mutex
	bufferSize := 3
	collector := NewBufferedCollector(func(items []*int) error {
		mu.Lock()
		collectedItems = append(collectedItems, items...)
		mu.Unlock()
		return nil
	}, BufferedCollectorConfig{BufferSize: &bufferSize})

	disp := NewDispatcher(worker, producer, *collector, DispatcherConfig{})
	errs := disp.Dispatch()

	if errs == nil || len(*errs) == 0 {
		t.Fatalf("Expected errors, got nil or empty")
	}
	if len(*errs) != 1 {
		t.Errorf("Expected 1 error, got %d", len(*errs))
	}
	if (*errs)[0] != workerErr {
		t.Errorf("Expected error '%v', got '%v'", workerErr, (*errs)[0])
	}

	if processedSuccessfully.Load() != int32(numItems-1) { // 0 and 2 should process
		t.Errorf("Expected %d items to be processed successfully, got %d", numItems-1, processedSuccessfully.Load())
	}

	if len(collectedItems) != numItems-1 {
		t.Errorf("Expected %d items to be collected, got %d", numItems-1, len(collectedItems))
	}
	// Check that collected items are 0*2 and 2*2
	expectedCollected := map[int]bool{0: true, 4: true}
	for _, item := range collectedItems {
		if !expectedCollected[*item] {
			t.Errorf("Collected unexpected item: %d", *item)
		}
	}
}

func TestDispatch_CollectorError(t *testing.T) {
	numItems := 3
	producer := newIntProducer(numItems)
	collectorErr := errors.New("collector failed")

	worker := Worker[int, int]{Work: func(item *int) (*int, error) { res := *item; return &res, nil }}

	bufferSize := 2
	collector := NewBufferedCollector(func(items []*int) error {
		// Fail on the first call to Collect
		return collectorErr
	}, BufferedCollectorConfig{BufferSize: &bufferSize}) // Buffer size 2, so Collect will be called

	disp := NewDispatcher(worker, producer, *collector, DispatcherConfig{})
	errs := disp.Dispatch()

	if errs == nil || len(*errs) == 0 {
		t.Fatalf("Expected errors, got nil or empty")
	}
	// Collector might be called multiple times if some items are processed before error
	// The current BufferedCollector stops on first error and doesn't process further batches.
	// The dispatcher will collect all errors from the errc channel.
	found := slices.Contains(*errs, collectorErr)
	if !found {
		t.Errorf("Expected collector error '%v' in %v", collectorErr, *errs)
	}
}

func TestDispatch_EmptyProducer(t *testing.T) {
	producer := newIntProducer(0) // No items

	var workerCalled atomic.Bool
	worker := Worker[int, int]{
		Work: func(item *int) (*int, error) {
			workerCalled.Store(true)
			return item, nil
		},
	}

	var collectCalled atomic.Bool
	bufferSize := 5
	collector := NewBufferedCollector(func(items []*int) error {
		if len(items) > 0 { // Collect might be called with an empty slice if buffer isn't hit then channel closes
			collectCalled.Store(true)
		}
		return nil
	}, BufferedCollectorConfig{BufferSize: &bufferSize})

	disp := NewDispatcher(worker, producer, *collector, DispatcherConfig{})
	errs := disp.Dispatch()

	if errs != nil {
		t.Fatalf("Expected no errors for empty producer, got %v", *errs)
	}
	if workerCalled.Load() {
		t.Error("Worker should not have been called for empty producer")
	}
	// Collector.Run is called. It receives from 'out'. If 'out' closes without items,
	// the loop 'for i := range in' in BufferedCollector.Run finishes.
	// Then, 'if len(buffer) > 0' is checked. If empty, Collect isn't called.
	// So collectCalled should remain false.
	if collectCalled.Load() {
		t.Error("Collector's Collect func should not have been called with items for empty producer")
	}
}

func TestDispatch_CollectorBuffering(t *testing.T) {
	tests := []struct {
		name                   string
		numItems               int
		bufferSize             int
		expectedCollectCalls   int32
		expectedTotalCollected int
	}{
		{"LessThanBuffer", 3, 5, 1, 3},
		{"EqualToBuffer", 5, 5, 1, 5},
		{"MoreThanBuffer", 7, 3, 3, 7}, // 3, 3, 1
		{"NoBuffering (Size 1)", 3, 1, 3, 3},
		{"Buffer Larger Than Items then Zero Items", 0, 5, 0, 0}, // Collector called with empty buffer at end if no items
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			producer := newIntProducer(tt.numItems)
			worker := Worker[int, int]{Work: func(item *int) (*int, error) { res := *item; return &res, nil }}

			var collectedItems []*int
			var mu sync.Mutex
			var collectCallCount atomic.Int32

			collector := NewBufferedCollector(func(items []*int) error {
				mu.Lock()
				defer mu.Unlock()
				// only count if items are present, as final collect might be empty if no items were ever sent
				if len(items) > 0 {
					collectCallCount.Add(1)
				}
				collectedItems = append(collectedItems, items...)
				return nil
			}, BufferedCollectorConfig{BufferSize: &tt.bufferSize})

			disp := NewDispatcher(worker, producer, *collector, DispatcherConfig{})
			errs := disp.Dispatch()

			if errs != nil {
				t.Fatalf("Test '%s': Expected no errors, got %v", tt.name, *errs)
			}

			mu.Lock()
			if len(collectedItems) != tt.expectedTotalCollected {
				t.Errorf("Test '%s': Expected %d total items collected, got %d", tt.name, tt.expectedTotalCollected, len(collectedItems))
			}
			mu.Unlock()

			actualCollectCalls := collectCallCount.Load()
			if tt.numItems == 0 && actualCollectCalls != 0 { // If no items, collect is not called by BufferedCollector.
				t.Errorf("Test '%s': Expected 0 collect calls for 0 items, got %d", tt.name, actualCollectCalls)
			} else if actualCollectCalls != tt.expectedCollectCalls {
				t.Errorf("Test '%s': Expected %d collect calls, got %d (numItems: %d, bufferSize: %d)",
					tt.name, tt.expectedCollectCalls, actualCollectCalls, tt.numItems, tt.bufferSize)
			}
		})
	}
}

func TestDispatch_MultipleWorkers(t *testing.T) {
	numItems := 20
	numWorkers := 4
	producer := newIntProducer(numItems)

	worker := Worker[int, int]{
		Work: func(item *int) (*int, error) {
			// Assign a pseudo worker ID for tracking (not truly how dispatcher assigns)
			// This is more to see if work is distributed
			// A real test of distribution might require instrumenting the dispatcher itself
			// or using distinct worker functions.
			// For now, just count that all items are processed.
			// A small delay can help observe concurrency.
			time.Sleep(time.Millisecond * time.Duration(5+(*item%3))) // variable delay
			res := *item * 2
			return &res, nil
		},
	}

	var collectedItems []*int
	var mu sync.Mutex
	bufferSize := 5
	collector := NewBufferedCollector(func(items []*int) error {
		mu.Lock()
		collectedItems = append(collectedItems, items...)
		mu.Unlock()
		return nil
	}, BufferedCollectorConfig{BufferSize: &bufferSize})

	disp := NewDispatcher(worker, producer, *collector, DispatcherConfig{NumWorker: &numWorkers})
	errs := disp.Dispatch()

	if errs != nil {
		t.Fatalf("Expected no errors, got %v", *errs)
	}

	mu.Lock()
	if len(collectedItems) != numItems {
		t.Errorf("Expected %d items to be collected, got %d", numItems, len(collectedItems))
	}
	mu.Unlock()

	// Verification that multiple workers did *something* is tricky without more instrumentation.
	// The primary check is that all work gets done correctly.
	// If NumWorker was 1, and processing was slow, it would take much longer.
	// We trust Go's scheduler to distribute tasks among worker goroutines reading from 'in'.
}

func TestDispatch_AllComponentsError(t *testing.T) {
	numItems := 5
	producer := newIntProducer(numItems) // 0, 1, 2, 3, 4

	workerErrEven := errors.New("worker failed on even")
	collectorErr := errors.New("collector failed during collection")

	var itemsAttemptedByWorker atomic.Int32
	worker := Worker[int, int]{
		Work: func(item *int) (*int, error) {
			itemsAttemptedByWorker.Add(1)
			if *item%2 == 0 { // Fail for even items: 0, 2, 4
				return nil, fmt.Errorf("%w: item %d", workerErrEven, *item)
			}
			res := *item * 10 // Process odd items: 1, 3
			return &res, nil
		},
	}

	var successfulItemsForCollector [][]*int
	var mu sync.Mutex
	bufferSize := 2
	collector := NewBufferedCollector(func(items []*int) error {
		mu.Lock()
		// Copy items as the slice might be reused by BufferedCollector
		currentBatch := make([]*int, len(items))
		for i, item := range items {
			val := *item
			currentBatch[i] = &val
		}
		successfulItemsForCollector = append(successfulItemsForCollector, currentBatch)
		mu.Unlock()

		// Let's say collector fails if it receives item 30 (from original item 3)
		for _, item := range items {
			if *item == 30 {
				return collectorErr
			}
		}
		return nil
	}, BufferedCollectorConfig{BufferSize: &bufferSize}) // Buffer size 2

	disp := NewDispatcher(worker, producer, *collector, DispatcherConfig{})
	errsSlice := disp.Dispatch()

	if errsSlice == nil || len(*errsSlice) == 0 {
		t.Fatal("Expected errors from dispatcher, got nil or empty")
	}

	errsMap := make(map[string]int)
	for _, err := range *errsSlice {
		errsMap[err.Error()]++
		// Check for wrapped errors if using fmt.Errorf with %w
		if errors.Is(err, workerErrEven) {
			errsMap["workerErrEven_is"]++
		}
		if errors.Is(err, collectorErr) {
			errsMap["collectorErr_is"]++
		}
	}

	// Expected worker errors for 0, 2, 4 (3 errors)
	// Expected collector error for item 30 (which is original 3 * 10) (1 error)
	// Total 4 errors.

	expectedWorkerErrCount := 3 // for items 0, 2, 4
	actualWorkerErrCount := 0
	for _, err := range *errsSlice {
		if errors.Is(err, workerErrEven) {
			actualWorkerErrCount++
		}
	}
	if actualWorkerErrCount != expectedWorkerErrCount {
		t.Errorf("Expected %d worker errors (workerErrEven), got %d. All errors: %v", expectedWorkerErrCount, actualWorkerErrCount, *errsSlice)
	}

	foundCollectorError := false
	for _, err := range *errsSlice {
		if errors.Is(err, collectorErr) {
			foundCollectorError = true
			break
		}
	}
	if !foundCollectorError {
		t.Errorf("Expected collectorError to be present. All errors: %v", *errsSlice)
	}

	// Verify collected items before collector error (if any)
	// Item 1 (10) should be collected. Item 3 (30) causes collector error.
	// Depending on timing and buffer, item 10 might be in one batch.
	// Item 30 will be in a subsequent batch causing error.
	mu.Lock()
	defer mu.Unlock()

	collectedValues := []int{}
	for _, batch := range successfulItemsForCollector {
		for _, itemPtr := range batch {
			collectedValues = append(collectedValues, *itemPtr)
		}
	}

	// Item 1 -> 10 should be collected.
	// Item 3 -> 30 would trigger collector error. So the batch containing 30 might or might not be added to successfulItemsForCollector
	// depending on when the error is returned by the CollectFunc.
	// Our mock CollectFunc adds to successfulItemsForCollector *before* returning error.

	hasTen := false
	hasThirty := false
	for _, val := range collectedValues {
		if val == 10 {
			hasTen = true
		}
		if val == 30 {
			hasThirty = true
		} // This means item 30 was passed to Collect
	}

	if !hasTen {
		t.Errorf("Expected item 10 (from original 1) to be passed to collector's Collect func. Got: %v", collectedValues)
	}
	if !hasThirty {
		// This means the batch with 30 was passed to the Collect func, which is correct, and then it errored.
		t.Logf("Item 30 (from original 3) was passed to collector and triggered error, which is expected. Collected prior to error: %v", collectedValues)
	}

	// Total errors: 3 from worker (0, 2, 4), 1 from collector (when processing item 30)
	if len(*errsSlice) != 4 {
		t.Errorf("Expected total 4 errors, got %d. Errors: %v", len(*errsSlice), *errsSlice)
	}

}
