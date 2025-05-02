package beehive

// import (
// 	"errors"
// 	"fmt"
// 	"slices" // Requires Go 1.21+
// 	"sync"
// 	"testing"
// 	"time"
// )

// // Mock Collector Functionality
// type mockCollectStats struct {
// 	mu          sync.Mutex
// 	calls       [][]*int // Record the buffer passed in each call
// 	returnError error    // Error to return on next call (if set)
// 	callCount   int
// }

// func (m *mockCollectStats) Collect(buffer []*int) error {
// 	m.mu.Lock()
// 	defer m.mu.Unlock()

// 	m.callCount++
// 	// Deep copy the buffer to avoid issues with slice reuse
// 	bufCopy := make([]*int, len(buffer))
// 	for i, ptr := range buffer {
// 		if ptr != nil {
// 			val := *ptr
// 			bufCopy[i] = &val
// 		} else {
// 			bufCopy[i] = nil // Handle potential nil pointers if necessary
// 		}
// 	}
// 	m.calls = append(m.calls, bufCopy)

// 	err := m.returnError
// 	m.returnError = nil // Reset error after returning it once
// 	return err
// }

// func (m *mockCollectStats) CallCount() int {
// 	m.mu.Lock()
// 	defer m.mu.Unlock()
// 	return m.callCount
// }

// func (m *mockCollectStats) GetCallArgs(callIndex int) ([]*int, error) {
// 	m.mu.Lock()
// 	defer m.mu.Unlock()
// 	if callIndex < 0 || callIndex >= len(m.calls) {
// 		return nil, fmt.Errorf("call index %d out of bounds (total calls: %d)", callIndex, len(m.calls))
// 	}
// 	return m.calls[callIndex], nil
// }

// func compareIntPointerSlices(t *testing.T, expected []int, actual []*int) {
// 	t.Helper()
// 	if len(expected) != len(actual) {
// 		t.Errorf("Expected slice length %d, got %d", len(expected), len(actual))
// 		return
// 	}
// 	actualVals := make([]int, len(actual))
// 	for i, ptr := range actual {
// 		if ptr == nil {
// 			t.Errorf("Unexpected nil pointer in actual slice at index %d", i)
// 			return
// 		}
// 		actualVals[i] = *ptr
// 	}
// 	// Sort both to compare content regardless of order within batch if needed
// 	// slices.Sort(expected)
// 	// slices.Sort(actualVals)
// 	if !slices.Equal(expected, actualVals) {
// 		t.Errorf("Expected slice %v, got %v", expected, actualVals)
// 	}
// }

// // --- Test Cases ---

// func TestCollector_NewBufferedCollectorDefaults(t *testing.T) {
// 	t.Parallel()
// 	mockStats := &mockCollectStats{}

// 	// Nil config
// 	c1 := NewBufferedCollector(mockStats.Collect, BufferedCollectorConfig{})
// 	if c1.BufferSize != 1 {
// 		t.Errorf("Expected default BufferSize 1 with empty config, got %d", c1.BufferSize)
// 	}

// 	// Nil BufferSize field
// 	c2 := NewBufferedCollector(mockStats.Collect, BufferedCollectorConfig{BufferSize: nil})
// 	if c2.BufferSize != 1 {
// 		t.Errorf("Expected default BufferSize 1 with nil BufferSize, got %d", c2.BufferSize)
// 	}

// 	// BufferSize 0 or 1
// 	bs0 := 0
// 	c3 := NewBufferedCollector(mockStats.Collect, BufferedCollectorConfig{BufferSize: &bs0})
// 	if c3.BufferSize != 1 {
// 		t.Errorf("Expected BufferSize 1 when config provides 0, got %d", c3.BufferSize)
// 	}
// 	bs1 := 1
// 	c4 := NewBufferedCollector(mockStats.Collect, BufferedCollectorConfig{BufferSize: &bs1})
// 	if c4.BufferSize != 1 {
// 		t.Errorf("Expected BufferSize 1 when config provides 1, got %d", c4.BufferSize)
// 	}

// 	// BufferSize > 1
// 	bs5 := 5
// 	c5 := NewBufferedCollector(mockStats.Collect, BufferedCollectorConfig{BufferSize: &bs5})
// 	if c5.BufferSize != 5 {
// 		t.Errorf("Expected BufferSize 5 when config provides 5, got %d", c5.BufferSize)
// 	}
// }

// func runCollectorTest(t *testing.T, bufferSize int, itemsToSend []int, mockStats *mockCollectStats) []error {
// 	t.Helper()
// 	in := make(chan *int)
// 	errc := make(chan error, len(itemsToSend)) // Buffer potential errors
// 	var wg sync.WaitGroup

// 	collector := BufferedCollector[int]{
// 		Collect:    mockStats.Collect,
// 		BufferSize: bufferSize,
// 	}
// 	// Correction: Ensure the collector uses the intended buffer size
// 	if bufferSize <= 1 {
// 		collector.BufferSize = 1
// 	}

// 	wg.Add(1)
// 	go collector.Run(in, errc, &wg)

// 	// Send items
// 	go func() {
// 		for i := range itemsToSend {
// 			v := itemsToSend[i]
// 			in <- &v
// 		}
// 		close(in) // Signal end of input
// 	}()

// 	// Wait for collector with timeout
// 	waitChan := make(chan struct{})
// 	go func() {
// 		wg.Wait()
// 		close(waitChan)
// 	}()

// 	select {
// 	case <-waitChan:
// 		// Collector finished
// 	case <-time.After(200 * time.Millisecond): // Adjust timeout as needed
// 		t.Fatalf("Collector did not signal Done within timeout (BufferSize: %d, Items: %d)", bufferSize, len(itemsToSend))
// 	}

// 	close(errc)
// 	stopSignal := make(chan struct{})
// 	close(stopSignal)
// 	return collectErrors(errc, stopSignal)
// }

// func TestCollector_BufferSizeOne(t *testing.T) {
// 	t.Parallel()
// 	mockStats := &mockCollectStats{}
// 	items := []int{10, 20, 30}

// 	errs := runCollectorTest(t, 1, items, mockStats)

// 	if len(errs) > 0 {
// 		t.Errorf("Unexpected errors: %v", errs)
// 	}
// 	if mockStats.CallCount() != 3 {
// 		t.Errorf("Expected Collect to be called 3 times, got %d", mockStats.CallCount())
// 	}
// 	// Check args for each call
// 	args0, _ := mockStats.GetCallArgs(0)
// 	compareIntPointerSlices(t, []int{10}, args0)
// 	args1, _ := mockStats.GetCallArgs(1)
// 	compareIntPointerSlices(t, []int{20}, args1)
// 	args2, _ := mockStats.GetCallArgs(2)
// 	compareIntPointerSlices(t, []int{30}, args2)
// }

// func TestCollector_BufferSizeMultiple_FullBuffer(t *testing.T) {
// 	t.Parallel()
// 	mockStats := &mockCollectStats{}
// 	items := []int{1, 2, 3}

// 	errs := runCollectorTest(t, 3, items, mockStats)

// 	if len(errs) > 0 {
// 		t.Errorf("Unexpected errors: %v", errs)
// 	}
// 	if mockStats.CallCount() != 1 {
// 		t.Errorf("Expected Collect to be called 1 time, got %d", mockStats.CallCount())
// 	}
// 	args0, _ := mockStats.GetCallArgs(0)
// 	compareIntPointerSlices(t, []int{1, 2, 3}, args0)
// }

// func TestCollector_BufferSizeMultiple_PartialBufferThenClose(t *testing.T) {
// 	t.Parallel()
// 	mockStats := &mockCollectStats{}
// 	items := []int{1, 2}

// 	errs := runCollectorTest(t, 3, items, mockStats)

// 	if len(errs) > 0 {
// 		t.Errorf("Unexpected errors: %v", errs)
// 	}
// 	if mockStats.CallCount() != 1 { // Only called on final flush
// 		t.Errorf("Expected Collect to be called 1 time, got %d", mockStats.CallCount())
// 	}
// 	args0, _ := mockStats.GetCallArgs(0)
// 	compareIntPointerSlices(t, []int{1, 2}, args0)
// }

// func TestCollector_BufferSizeMultiple_MultipleBatchesAndRemainder(t *testing.T) {
// 	t.Parallel()
// 	mockStats := &mockCollectStats{}
// 	items := []int{1, 2, 3, 4, 5, 6, 7} // 2 full batches + 1 remainder

// 	errs := runCollectorTest(t, 3, items, mockStats)

// 	if len(errs) > 0 {
// 		t.Errorf("Unexpected errors: %v", errs)
// 	}
// 	if mockStats.CallCount() != 3 {
// 		t.Errorf("Expected Collect to be called 3 times, got %d", mockStats.CallCount())
// 	}
// 	// Check call args
// 	args0, _ := mockStats.GetCallArgs(0)
// 	compareIntPointerSlices(t, []int{1, 2, 3}, args0)
// 	args1, _ := mockStats.GetCallArgs(1)
// 	compareIntPointerSlices(t, []int{4, 5, 6}, args1)
// 	args2, _ := mockStats.GetCallArgs(2)
// 	compareIntPointerSlices(t, []int{7}, args2) // Remainder
// }

// func TestCollector_EmptyInput(t *testing.T) {
// 	t.Parallel()
// 	mockStats := &mockCollectStats{}
// 	items := []int{} // Empty input

// 	errs := runCollectorTest(t, 3, items, mockStats)

// 	if len(errs) > 0 {
// 		t.Errorf("Unexpected errors: %v", errs)
// 	}
// 	if mockStats.CallCount() != 0 { // Should not be called
// 		t.Errorf("Expected Collect to be called 0 times, got %d", mockStats.CallCount())
// 	}
// }

// func TestCollector_CollectError(t *testing.T) {
// 	t.Parallel()
// 	mockStats := &mockCollectStats{}
// 	testErr := errors.New("collection failed")
// 	mockStats.returnError = testErr // Set error for the first call

// 	items := []int{1, 2, 3, 4} // BufferSize=2 -> Error on {1, 2}, collect {3, 4}

// 	errs := runCollectorTest(t, 2, items, mockStats)

// 	if len(errs) != 1 {
// 		t.Fatalf("Expected 1 error, got %d: %v", len(errs), errs)
// 	}
// 	if !errors.Is(errs[0], testErr) {
// 		t.Errorf("Expected error '%v', got '%v'", testErr, errs[0])
// 	}
// 	// Check it still processed subsequent batches
// 	if mockStats.CallCount() != 2 {
// 		t.Errorf("Expected Collect to be called 2 times (once failing, once succeeding), got %d", mockStats.CallCount())
// 	}
// 	args1, _ := mockStats.GetCallArgs(1) // Second call args
// 	compareIntPointerSlices(t, []int{3, 4}, args1)
// }

// func TestCollector_SignalsDone(t *testing.T) {
// 	// This is implicitly tested by runCollectorTest completing successfully
// 	// within the timeout. We can make it explicit if needed, but wg.Wait()
// 	// failing the timeout in runCollectorTest covers this.
// 	t.Skip("Implicitly tested by runCollectorTest finishing")
// }
