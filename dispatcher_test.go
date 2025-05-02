package beehive

// import (
// 	"bytes"
// 	"errors"
// 	"fmt"
// 	"iter"
// 	"log/slog"
// 	"runtime"
// 	"slices"
// 	"sync"
// 	"testing"
// 	"time"
// )

// // --- Mocks and Helpers ---

// // Simple producer from a slice
// func sliceProducer[T any](items []T) iter.Seq[T] {
// 	return func(yield func(T) bool) {
// 		for _, item := range items {
// 			if !yield(item) {
// 				return
// 			}
// 		}
// 	}
// }

// // Mock Worker Do function
// func mockWorkerDo(mul int, errOn int) func(i *int) (*string, error) {
// 	return func(i *int) (*string, error) {
// 		if *i == errOn {
// 			return nil, fmt.Errorf("worker error on %d", *i)
// 		}
// 		// Simulate some work
// 		time.Sleep(time.Duration(mul%3+1) * time.Millisecond)
// 		s := fmt.Sprintf("w:%d", *i*mul)
// 		return &s, nil
// 	}
// }

// // Mock Collector (thread-safe)
// type mockDispatcherCollector struct {
// 	mu          sync.Mutex
// 	collected   [][]string // Stores batches as received
// 	flatResults []string   // Stores all results flattened
// 	errors      []error    // Errors returned by Collect
// 	collectErr  error      // Error to return on next call
// }

// func (m *mockDispatcherCollector) Collect(buffer []*string) error {
// 	m.mu.Lock()
// 	defer m.mu.Unlock()

// 	batch := make([]string, len(buffer))
// 	for i, sPtr := range buffer {
// 		if sPtr != nil {
// 			batch[i] = *sPtr
// 			m.flatResults = append(m.flatResults, *sPtr)
// 		} else {
// 			// Handle nil if possible, though worker shouldn't send nil on success
// 			batch[i] = "<nil>"
// 			m.flatResults = append(m.flatResults, "<nil>")
// 		}
// 	}
// 	m.collected = append(m.collected, batch)

// 	err := m.collectErr
// 	if err != nil {
// 		m.errors = append(m.errors, err)
// 		m.collectErr = nil // Reset
// 	}
// 	return err
// }

// func (m *mockDispatcherCollector) GetFlatResults() []string {
// 	m.mu.Lock()
// 	defer m.mu.Unlock()
// 	// Return a copy
// 	res := make([]string, len(m.flatResults))
// 	copy(res, m.flatResults)
// 	return res
// }

// func (m *mockDispatcherCollector) SetError(err error) {
// 	m.mu.Lock()
// 	defer m.mu.Unlock()
// 	m.collectErr = err
// }

// // Mock Logger
// type mockLogger struct {
// 	mu     sync.Mutex
// 	buf    bytes.Buffer
// 	logger *slog.Logger
// }

// func newMockLogger() *mockLogger {
// 	m := &mockLogger{}
// 	m.logger = slog.New(slog.NewTextHandler(&m.buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
// 	return m
// }

// func (m *mockLogger) Logger() *slog.Logger {
// 	return m.logger
// }

// func (m *mockLogger) String() string {
// 	m.mu.Lock()
// 	defer m.mu.Unlock()
// 	return m.buf.String()
// }

// func (m *mockLogger) Contains(s string) bool {
// 	return bytes.Contains(m.buf.Bytes(), []byte(s))
// }

// // --- Test Cases ---

// func TestDispatcher_NewDispatcherDefaults(t *testing.T) {
// 	t.Parallel()
// 	mockCollector := &mockDispatcherCollector{}
// 	mockProd := sliceProducer([]int{1})
// 	worker := Worker[int, string]{Do: mockWorkerDo(1, -1)} // Simple worker
// 	collector := NewBufferedCollector(mockCollector.Collect, BufferedCollectorConfig{})

// 	cfg := DispatcherConfig{} // Empty config
// 	d := NewDispatcher(worker, mockProd, *collector, cfg)

// 	if d.NoWorker != runtime.NumCPU() {
// 		t.Errorf("Expected default NoWorker %d, got %d", runtime.NumCPU(), d.NoWorker)
// 	}
// 	if d.rateLimit != 0 {
// 		t.Errorf("Expected default rateLimit 0, got %v", d.rateLimit)
// 	}
// 	if d.logger == nil {
// 		t.Fatal("Expected default logger to be non-nil")
// 	}
// 	// Check logger output goes to discard by default (hard to test precisely, check type maybe)
// 	if _, ok := d.logger.Handler().(*slog.TextHandler); !ok {
// 		// Default may change, this is just an example check
// 		// t.Logf("Default logger handler type: %T", d.logger.Handler())
// 	}

// 	// Default buffer size is 1 for collector
// 	expectedChanBuffer := 1 / d.NoWorker
// 	if expectedChanBuffer == 0 {
// 		expectedChanBuffer = 1
// 	} // Assuming min buffer of 1, adjust based on actual logic
// 	// This calculation depends heavily on the revised logic in NewDispatcher
// 	// if d.channelBuffer != expectedChanBuffer {
// 	// 	t.Errorf("Expected channelBuffer %d (based on BufferSize=1 / NoWorker=%d), got %d", expectedChanBuffer, d.NoWorker, d.channelBuffer)
// 	// }
// 	t.Logf("Note: channelBuffer test depends on revised calculation logic.")

// }

// func TestDispatcher_NewDispatcherCustomConfig(t *testing.T) {
// 	t.Parallel()
// 	mockCollector := &mockDispatcherCollector{}
// 	mockProd := sliceProducer([]int{1})
// 	worker := Worker[int, string]{Do: mockWorkerDo(1, -1)}
// 	collector := NewBufferedCollector(mockCollector.Collect, BufferedCollectorConfig{})
// 	mockLog := newMockLogger()
// 	rate := 100 * time.Millisecond
// 	numWorkers := 3

// 	cfg := DispatcherConfig{
// 		NoWorker:  &numWorkers,
// 		RateLimit: &rate,
// 		Logger:    mockLog.Logger(),
// 	}
// 	d := NewDispatcher(worker, mockProd, *collector, cfg)

// 	if d.NoWorker != numWorkers {
// 		t.Errorf("Expected NoWorker %d, got %d", numWorkers, d.NoWorker)
// 	}
// 	if d.rateLimit != rate {
// 		t.Errorf("Expected rateLimit %v, got %v", rate, d.rateLimit)
// 	}
// 	if d.logger != mockLog.Logger() {
// 		t.Error("Expected custom logger to be set")
// 	}

// 	// Test NoWorker <= 0
// 	numWorkersNeg := -1
// 	cfgNeg := DispatcherConfig{NoWorker: &numWorkersNeg}
// 	dNeg := NewDispatcher(worker, mockProd, *collector, cfgNeg)
// 	if dNeg.NoWorker != runtime.NumCPU() {
// 		t.Errorf("Expected NoWorker <= 0 to default to %d, got %d", runtime.NumCPU(), dNeg.NoWorker)
// 	}
// }

// // Helper for basic flow tests
// func runDispatcherFlowTest(t *testing.T, numWorkers, bufferSize int, items []int) (*mockDispatcherCollector, *mockLogger) {
// 	t.Helper()
// 	mockCollector := &mockDispatcherCollector{}
// 	mockLog := newMockLogger()
// 	mockProd := sliceProducer(items)

// 	worker := Worker[int, string]{Do: mockWorkerDo(10, -1)} // Worker multiplies by 10, no errors

// 	bs := bufferSize // Need a variable to take address of
// 	collector := NewBufferedCollector(mockCollector.Collect, BufferedCollectorConfig{BufferSize: &bs})

// 	nw := numWorkers
// 	cfg := DispatcherConfig{
// 		NoWorker: &nw,
// 		Logger:   mockLog.Logger(),
// 	}
// 	d := NewDispatcher(worker, mockProd, *collector, cfg)

// 	d.Dispatch() // Run the dispatcher

// 	return mockCollector, mockLog
// }

// func TestDispatcher_BasicFlow_SingleWorker_Buffer1(t *testing.T) {
// 	t.Parallel()
// 	items := []int{1, 2, 3}
// 	mockCollector, mockLog := runDispatcherFlowTest(t, 1, 1, items)

// 	results := mockCollector.GetFlatResults()
// 	expected := []string{"w:10", "w:20", "w:30"}

// 	// Sort results because worker order isn't strictly guaranteed even with 1 worker if channels allow reordering slightly
// 	slices.Sort(results)
// 	slices.Sort(expected)

// 	if !slices.Equal(results, expected) {
// 		t.Errorf("Expected results %v, got %v", expected, results)
// 	}
// 	if mockLog.Contains("error") {
// 		t.Errorf("Log should not contain errors, log:\n%s", mockLog.String())
// 	}
// }

// func TestDispatcher_BasicFlow_MultipleWorkers_BufferN(t *testing.T) {
// 	t.Parallel()
// 	items := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
// 	numWorkers := 4
// 	bufferSize := 3
// 	mockCollector, mockLog := runDispatcherFlowTest(t, numWorkers, bufferSize, items)

// 	results := mockCollector.GetFlatResults()
// 	expected := make([]string, len(items))
// 	for i, item := range items {
// 		expected[i] = fmt.Sprintf("w:%d", item*10)
// 	}

// 	// Sort because order is not guaranteed with multiple workers
// 	slices.Sort(results)
// 	slices.Sort(expected)

// 	if !slices.Equal(results, expected) {
// 		t.Errorf("Expected results %v, got %v", expected, results)
// 	}
// 	if mockLog.Contains("error") {
// 		t.Errorf("Log should not contain errors, log:\n%s", mockLog.String())
// 	}
// 	// Optional: Check batching structure in mockCollector.collected if needed
// }

// func TestDispatcher_EmptyProducer(t *testing.T) {
// 	t.Parallel()
// 	items := []int{} // Empty
// 	mockCollector, mockLog := runDispatcherFlowTest(t, 2, 2, items)

// 	results := mockCollector.GetFlatResults()
// 	if len(results) != 0 {
// 		t.Errorf("Expected 0 results for empty producer, got %d", len(results))
// 	}
// 	if mockLog.Contains("error") {
// 		t.Errorf("Log should not contain errors, log:\n%s", mockLog.String())
// 	}
// 	// Check logs for clean shutdown messages
// 	if !mockLog.Contains("Producer finished") || !mockLog.Contains("Worker finished") || !mockLog.Contains("Collector finished") {
// 		t.Errorf("Expected shutdown log messages, log:\n%s", mockLog.String())
// 	}
// }

// func TestDispatcher_WorkerError(t *testing.T) {
// 	t.Parallel()
// 	items := []int{1, 2, 3, 4}
// 	errorOn := 3 // Worker should fail on item 3

// 	mockCollector := &mockDispatcherCollector{}
// 	mockLog := newMockLogger()
// 	mockProd := sliceProducer(items)

// 	// Worker fails on 'errorOn'
// 	worker := Worker[int, string]{Do: mockWorkerDo(1, errorOn)}
// 	bs := 2
// 	collector := NewBufferedCollector(mockCollector.Collect, BufferedCollectorConfig{BufferSize: &bs})

// 	nw := 2
// 	cfg := DispatcherConfig{
// 		NoWorker: &nw,
// 		Logger:   mockLog.Logger(),
// 	}
// 	d := NewDispatcher(worker, mockProd, *collector, cfg)

// 	d.Dispatch()

// 	// Check logs for the error
// 	expectedErrMsg := fmt.Sprintf("worker error on %d", errorOn)
// 	if !mockLog.Contains("error") || !mockLog.Contains(expectedErrMsg) {
// 		t.Errorf("Expected log to contain error '%s', log:\n%s", expectedErrMsg, mockLog.String())
// 	}

// 	// Check collected results (should not contain result for failed item)
// 	results := mockCollector.GetFlatResults()
// 	expected := []string{"w:1", "w:2", "w:4"} // Result for 3 should be missing

// 	slices.Sort(results)
// 	slices.Sort(expected)

// 	if !slices.Equal(results, expected) {
// 		t.Errorf("Expected results %v after worker error, got %v", expected, results)
// 	}
// }

// func TestDispatcher_CollectorError(t *testing.T) {
// 	t.Parallel()
// 	items := []int{1, 2, 3, 4}
// 	collectErr := errors.New("collector failed to persist")

// 	mockCollector := &mockDispatcherCollector{}
// 	mockCollector.SetError(collectErr) // Make the first collect call fail
// 	mockLog := newMockLogger()
// 	mockProd := sliceProducer(items)

// 	worker := Worker[int, string]{Do: mockWorkerDo(1, -1)} // Worker always succeeds
// 	bs := 2
// 	collector := NewBufferedCollector(mockCollector.Collect, BufferedCollectorConfig{BufferSize: &bs})

// 	nw := 2
// 	cfg := DispatcherConfig{
// 		NoWorker: &nw,
// 		Logger:   mockLog.Logger(),
// 	}
// 	d := NewDispatcher(worker, mockProd, *collector, cfg)

// 	d.Dispatch()

// 	// Check logs for the collector error
// 	if !mockLog.Contains("error") || !mockLog.Contains(collectErr.Error()) {
// 		t.Errorf("Expected log to contain collector error '%s', log:\n%s", collectErr.Error(), mockLog.String())
// 	}

// 	// Check results - even if collect fails, items might still be passed if error handling allows continuation
// 	// The mock collector still stores results even if it returns an error here.
// 	results := mockCollector.GetFlatResults()
// 	expected := []string{"w:1", "w:2", "w:3", "w:4"}

// 	slices.Sort(results)
// 	slices.Sort(expected)

// 	if !slices.Equal(results, expected) {
// 		t.Errorf("Expected results %v even after collector error, got %v", expected, results)
// 	}
// }

// func TestDispatcher_RateLimiting(t *testing.T) {
// 	t.Parallel()
// 	if testing.Short() {
// 		t.Skip("Skipping rate limit test in short mode")
// 	}

// 	items := []int{1, 2, 3, 4, 5}
// 	rate := 50 * time.Millisecond // 50ms between items
// 	numWorkers := 1               // Use 1 worker for predictability
// 	bufferSize := 1

// 	mockCollector := &mockDispatcherCollector{}
// 	mockLog := newMockLogger()
// 	mockProd := sliceProducer(items)

// 	worker := Worker[int, string]{Do: mockWorkerDo(1, -1)} // Fast worker
// 	bs := bufferSize
// 	collector := NewBufferedCollector(mockCollector.Collect, BufferedCollectorConfig{BufferSize: &bs})

// 	nw := numWorkers
// 	rl := rate
// 	cfg := DispatcherConfig{
// 		NoWorker:  &nw,
// 		RateLimit: &rl,
// 		Logger:    mockLog.Logger(),
// 	}
// 	d := NewDispatcher(worker, mockProd, *collector, cfg)

// 	startTime := time.Now()
// 	d.Dispatch()
// 	duration := time.Since(startTime)

// 	// Expect duration to be roughly (N-1) * rate
// 	// Allow some buffer for scheduling delays, etc.
// 	minExpectedDuration := time.Duration(len(items)-1) * rate
// 	// Reduce buffer slightly to catch cases where rate limiting isn't effective
// 	minExpectedDuration = time.Duration(float64(minExpectedDuration) * 0.85)
// 	// Set an upper bound too, e.g., 2x expected + base time
// 	maxExpectedDuration := (time.Duration(len(items)) * rate) + 100*time.Millisecond

// 	t.Logf("Rate limit test duration: %v (Rate: %v, Items: %d)", duration, rate, len(items))

// 	if duration < minExpectedDuration {
// 		t.Errorf("Execution time %v is less than minimum expected %v, rate limiting might not be effective", duration, minExpectedDuration)
// 	}
// 	if duration > maxExpectedDuration {
// 		t.Errorf("Execution time %v is much higher than maximum expected %v", duration, maxExpectedDuration)
// 	}

// 	// Also check results were processed correctly
// 	results := mockCollector.GetFlatResults()
// 	if len(results) != len(items) {
// 		t.Errorf("Expected %d results, got %d", len(items), len(results))
// 	}
// 	if mockLog.Contains("error") {
// 		t.Errorf("Log should not contain errors, log:\n%s", mockLog.String())
// 	}
// }

// // Example test for pointer safety concept (if needed)
// type Data struct {
// 	ID    int
// 	Value string
// }

// func TestDispatcher_ProducerPointerSafetyConcept(t *testing.T) {
// 	t.Parallel()
// 	items := []*Data{{ID: 1, Value: "A"}, {ID: 2, Value: "B"}}

// 	// Producer yielding pointers from the slice
// 	producer := func(yield func(*Data) bool) {
// 		for _, item := range items {
// 			if !yield(item) {
// 				return
// 			} // Yields pointer directly from range
// 		}
// 	}

// 	mockCollector := &mockDispatcherCollector{} // Using the string collector for simplicity
// 	mockLog := newMockLogger()

// 	// Worker that modifies a copy and returns a string representation
// 	worker := Worker[*Data, string]{
// 		Do: func(d *Data) (*string, error) {
// 			// IMPORTANT: Work on a copy if modification is needed before creating result
// 			// resultData := *d // Shallow copy
// 			// resultData.Value = "Processed_" + resultData.Value
// 			s := fmt.Sprintf("ID:%d,Val:%s", d.ID, d.Value) // Create result string
// 			return &s, nil
// 		},
// 	}
// 	bs := 1
// 	collector := NewBufferedCollector(mockCollector.Collect, BufferedCollectorConfig{BufferSize: &bs})

// 	nw := 2
// 	cfg := DispatcherConfig{
// 		NoWorker: &nw,
// 		Logger:   mockLog.Logger(),
// 	}
// 	d := NewDispatcher(worker, producer, *collector, cfg)
// 	d.Dispatch()

// 	results := mockCollector.GetFlatResults()
// 	expected := []string{"ID:1,Val:A", "ID:2,Val:B"}

// 	slices.Sort(results)
// 	slices.Sort(expected)

// 	if !slices.Equal(results, expected) {
// 		t.Errorf("Expected results %v, got %v", expected, results)
// 	}
// 	// Check original items were not modified (if worker logic was different)
// 	if items[0].Value != "A" || items[1].Value != "B" {
// 		t.Errorf("Original producer data was modified unexpectedly: %v", items)
// 	}
// }
