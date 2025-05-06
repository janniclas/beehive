package beehive

import (
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
)

// Mock Collector Functionality
type mockCollectStats struct {
	mu          sync.Mutex
	calls       [][]*int // Record the buffer passed in each call
	returnError error    // Error to return on next call (if set)
	callCount   int
}

func (m *mockCollectStats) Collect(buffer []*int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.callCount++
	// Deep copy the buffer to avoid issues with slice reuse
	bufCopy := make([]*int, len(buffer))
	for i, ptr := range buffer {
		if ptr != nil {
			val := *ptr
			bufCopy[i] = &val
		} else {
			bufCopy[i] = nil // Handle potential nil pointers if necessary
		}
	}
	m.calls = append(m.calls, bufCopy)

	err := m.returnError
	m.returnError = nil // Reset error after returning it once
	return err
}

func (m *mockCollectStats) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

func (m *mockCollectStats) GetCallArgs(callIndex int) ([]*int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if callIndex < 0 || callIndex >= len(m.calls) {
		return nil, fmt.Errorf("call index %d out of bounds (total calls: %d)", callIndex, len(m.calls))
	}
	return m.calls[callIndex], nil
}

func compareIntPointerSlices(t *testing.T, expected []int, actual []*int) {
	t.Helper()
	if len(expected) != len(actual) {
		t.Errorf("Expected slice length %d, got %d", len(expected), len(actual))
		return
	}
	actualVals := make([]int, len(actual))
	for i, ptr := range actual {
		if ptr == nil {
			t.Errorf("Unexpected nil pointer in actual slice at index %d", i)
			return
		}
		actualVals[i] = *ptr
	}
	if !slices.Equal(expected, actualVals) {
		t.Errorf("Expected slice %v, got %v", expected, actualVals)
	}
}

// // --- Test Cases ---

func TestCollector_NewBufferedCollectorDefaults(t *testing.T) {
	t.Parallel()
	mockStats := &mockCollectStats{}

	// Nil config
	c1 := NewBufferedCollector(mockStats.Collect, BufferedCollectorConfig{})
	if c1.BufferSize != 1 {
		t.Errorf("Expected default BufferSize 1 with empty config, got %d", c1.BufferSize)
	}

	// Nil BufferSize field
	c2 := NewBufferedCollector(mockStats.Collect, BufferedCollectorConfig{BufferSize: nil})
	if c2.BufferSize != 1 {
		t.Errorf("Expected default BufferSize 1 with nil BufferSize, got %d", c2.BufferSize)
	}

	// BufferSize 0 or 1
	bs0 := 0
	c3 := NewBufferedCollector(mockStats.Collect, BufferedCollectorConfig{BufferSize: &bs0})
	if c3.BufferSize != 1 {
		t.Errorf("Expected BufferSize 1 when config provides 0, got %d", c3.BufferSize)
	}
	bs1 := 1
	c4 := NewBufferedCollector(mockStats.Collect, BufferedCollectorConfig{BufferSize: &bs1})
	if c4.BufferSize != 1 {
		t.Errorf("Expected BufferSize 1 when config provides 1, got %d", c4.BufferSize)
	}

	// BufferSize > 1
	bs5 := 5
	c5 := NewBufferedCollector(mockStats.Collect, BufferedCollectorConfig{BufferSize: &bs5})
	if c5.BufferSize != 5 {
		t.Errorf("Expected BufferSize 5 when config provides 5, got %d", c5.BufferSize)
	}
}

func runCollectorTest(t *testing.T, bufferSize int, itemsToSend []int, mockStats *mockCollectStats) []error {
	t.Helper()
	in := make(chan *int)
	errc := make(chan error, 10) // buffer potential errors
	var wg sync.WaitGroup

	collector := BufferedCollector[int]{
		Collect:    mockStats.Collect,
		BufferSize: bufferSize,
	}

	wg.Add(1)
	go collector.Run(in, errc, &wg)

	// Send items
	go func() {
		for i := range itemsToSend {
			v := itemsToSend[i]
			in <- &v
		}
		close(in) // Signal end of input
	}()

	wg.Wait()

	close(errc)
	stopSignal := make(chan struct{})
	close(stopSignal)
	return collectErrors(errc)
}

func TestCollector_BufferSizeOne(t *testing.T) {
	t.Parallel()
	mockStats := &mockCollectStats{}
	items := []int{10, 20, 30}

	errs := runCollectorTest(t, 1, items, mockStats)

	if len(errs) > 0 {
		t.Errorf("Unexpected errors: %v", errs)
	}
	if mockStats.CallCount() != 3 {
		t.Errorf("Expected Collect to be called 3 times, got %d", mockStats.CallCount())
	}
	// Check args for each call
	args0, _ := mockStats.GetCallArgs(0)
	compareIntPointerSlices(t, []int{10}, args0)
	args1, _ := mockStats.GetCallArgs(1)
	compareIntPointerSlices(t, []int{20}, args1)
	args2, _ := mockStats.GetCallArgs(2)
	compareIntPointerSlices(t, []int{30}, args2)
}

func TestCollector_BufferSizeMultiple_FullBuffer(t *testing.T) {
	t.Parallel()
	mockStats := &mockCollectStats{}
	items := []int{1, 2, 3}

	errs := runCollectorTest(t, 3, items, mockStats)

	if len(errs) > 0 {
		t.Errorf("Unexpected errors: %v", errs)
	}
	if mockStats.CallCount() != 1 {
		t.Errorf("Expected Collect to be called 1 time, got %d", mockStats.CallCount())
	}
	args0, _ := mockStats.GetCallArgs(0)
	compareIntPointerSlices(t, []int{1, 2, 3}, args0)
}

func TestCollector_BufferSizeMultiple_PartialBufferThenClose(t *testing.T) {
	t.Parallel()
	mockStats := &mockCollectStats{}
	items := []int{1, 2}

	errs := runCollectorTest(t, 3, items, mockStats)

	if len(errs) > 0 {
		t.Errorf("Unexpected errors: %v", errs)
	}
	if mockStats.CallCount() != 1 { // Only called on final flush
		t.Errorf("Expected Collect to be called 1 time, got %d", mockStats.CallCount())
	}
	args0, _ := mockStats.GetCallArgs(0)
	compareIntPointerSlices(t, []int{1, 2}, args0)
}

func TestCollector_BufferSizeMultiple_MultipleBatchesAndRemainder(t *testing.T) {
	t.Parallel()
	mockStats := &mockCollectStats{}
	items := []int{1, 2, 3, 4, 5, 6, 7} // 2 full batches + 1 remainder

	errs := runCollectorTest(t, 3, items, mockStats)

	if len(errs) > 0 {
		t.Errorf("Unexpected errors: %v", errs)
	}
	if mockStats.CallCount() != 3 {
		t.Errorf("Expected Collect to be called 3 times, got %d", mockStats.CallCount())
	}
	// Check call args
	args0, _ := mockStats.GetCallArgs(0)
	compareIntPointerSlices(t, []int{1, 2, 3}, args0)
	args1, _ := mockStats.GetCallArgs(1)
	compareIntPointerSlices(t, []int{4, 5, 6}, args1)
	args2, _ := mockStats.GetCallArgs(2)
	compareIntPointerSlices(t, []int{7}, args2) // Remainder
}

func TestCollector_EmptyInput(t *testing.T) {
	t.Parallel()
	mockStats := &mockCollectStats{}
	items := []int{} // Empty input

	errs := runCollectorTest(t, 3, items, mockStats)

	if len(errs) > 0 {
		t.Errorf("Unexpected errors: %v", errs)
	}
	if mockStats.CallCount() != 0 { // Should not be called
		t.Errorf("Expected Collect to be called 0 times, got %d", mockStats.CallCount())
	}
}

func TestCollector_CollectError(t *testing.T) {
	t.Parallel()
	mockStats := &mockCollectStats{}
	testErr := errors.New("collection failed")
	mockStats.returnError = testErr // Set error for the first call

	items := []int{1, 2, 3, 4} // BufferSize=2 -> Error on {1, 2}, collect {3, 4}

	errs := runCollectorTest(t, 2, items, mockStats)

	if len(errs) != 1 {
		t.Fatalf("Expected 1 error, got %d: %v", len(errs), errs)
	}
	if !errors.Is(errs[0], testErr) {
		t.Errorf("Expected error '%v', got '%v'", testErr, errs[0])
	}
	// Check it still processed subsequent batches
	if mockStats.CallCount() != 2 {
		t.Errorf("Expected Collect to be called 2 times (once failing, once succeeding), got %d", mockStats.CallCount())
	}
	args1, _ := mockStats.GetCallArgs(1) // Second call args
	compareIntPointerSlices(t, []int{3, 4}, args1)
}

// TestCollector_ErrorOnFinalFlush tests the scenario where the input channel closes,
// there are remaining items in the buffer, and collecting these remaining items
// results in an error.
func TestCollector_ErrorOnFinalFlush(t *testing.T) {
	t.Parallel() // Mark test as safe for parallel execution

	mockStats := &mockCollectStats{}
	finalFlushErr := errors.New("error during final flush of remaining items")

	// Configure the mock to return an error on its (expected) only call.
	mockStats.returnError = finalFlushErr

	// Items that will remain in the buffer when 'in' is closed.
	itemsToFlush := []int{101, 102, 103}

	// Set BufferSize larger than the number of items to ensure they are
	// only processed during the final flush.
	bufferSize := 5

	// Ensure runCollectorTest uses a buffered error channel.
	// If your runCollectorTest was updated from the previous discussion, it should be fine.
	// For clarity, here's a reminder of how errc should be in runCollectorTest:
	// errc := make(chan error, 10) // Or some other buffer size > 0

	errs := runCollectorTest(t, bufferSize, itemsToFlush, mockStats)

	// --- Assertions ---

	// 1. Check that exactly one error was received.
	if len(errs) != 1 {
		t.Fatalf("Expected 1 error from final flush, got %d: %v", len(errs), errs)
	}

	// 2. Check that the received error is the one we expected.
	if !errors.Is(errs[0], finalFlushErr) {
		t.Errorf("Expected error '%v', got '%v'", finalFlushErr, errs[0])
	}

	// 3. Check that the Collect method was called exactly once.
	if mockStats.CallCount() != 1 {
		t.Errorf("Expected Collect to be called 1 time (for the final flush), got %d", mockStats.CallCount())
	}

	// 4. Check that Collect was called with the correct remaining items.
	args, err := mockStats.GetCallArgs(0) // Get arguments of the first (and only) call
	if err != nil {
		t.Fatalf("Error getting call arguments for Collect: %v", err)
	}
	compareIntPointerSlices(t, itemsToFlush, args)
}

// TestCollector_HandlesNilPointers tests that the collector can handle nil
// pointers within the input stream and includes them correctly in the batches
// passed to the Collect function.
func TestCollector_HandlesNilPointers(t *testing.T) {
	t.Parallel() // Mark test as safe for parallel execution

	mockStats := &mockCollectStats{} // mockCollectStats is already set up to handle nil pointers

	// Items including nil pointers at various positions
	// We use *int for the example, but the generic T allows any pointer type.
	itemsWithNil := []*int{intPtr(1), nil, intPtr(3), intPtr(4), nil, nil, intPtr(7)} // intPtr is a helper function to create *int
	bufferSize := 3                                                                   // Use a buffer size that will create multiple batches and a remainder

	// Helper to create a *int from an int value
	intPtr := func(v int) *int { return &v }

	// Transform itemsWithNil for the runCollectorTest helper
	// The helper expects []int and converts them to []*int.
	// We need to simulate the mixed input of int pointers and nil directly.
	// We will need a slightly modified run function or adjust the input sending.
	// Let's adjust the input sending part of a new test function.

	in := make(chan *int)
	errc := make(chan error, 10) // Buffer potential errors
	var wg sync.WaitGroup

	collector := BufferedCollector[int]{
		Collect:    mockStats.Collect,
		BufferSize: bufferSize,
	}

	wg.Add(1)
	go collector.Run(in, errc, &wg)

	// Send items including nil pointers
	go func() {
		for _, item := range itemsWithNil {
			in <- item
		}
		close(in) // Signal end of input
	}()

	wg.Wait()

	close(errc) // Close error channel after WaitGroup is done
	errs := collectErrors(errc)

	// --- Assertions ---

	// 1. Check that no unexpected errors occurred.
	if len(errs) > 0 {
		t.Errorf("Unexpected errors received: %v", errs)
	}

	// 2. Check the number of Collect calls.
	// itemsWithNil has 7 elements, bufferSize is 3.
	// Batch 1: [1, nil, 3] (call 1)
	// Batch 2: [4, nil, nil] (call 2)
	// Remainder: [7] (call 3 on flush)
	expectedCalls := 3
	if mockStats.CallCount() != expectedCalls {
		t.Errorf("Expected Collect to be called %d times, got %d", expectedCalls, mockStats.CallCount())
	}

	// 3. Check the arguments passed in each Collect call.
	// Helper to compare []*int including nil
	compareIntPointerSlicesWithNil := func(t *testing.T, expected []*int, actual []*int) {
		t.Helper()
		if len(expected) != len(actual) {
			t.Errorf("Expected slice length %d, got %d", len(expected), len(actual))
			return
		}
		for i := range expected {
			expectedNil := expected[i] == nil
			actualNil := actual[i] == nil

			if expectedNil != actualNil {
				t.Errorf("Mismatch at index %d: Expected nil: %t, Got nil: %t", i, expectedNil, actualNil)
				return // Stop on first mismatch
			}

			if !expectedNil { // If not nil, compare values
				if *expected[i] != *actual[i] {
					t.Errorf("Value mismatch at index %d: Expected %d, Got %d", i, *expected[i], *actual[i])
					return // Stop on first mismatch
				}
			}
		}
	}

	// Call 0
	args0, err := mockStats.GetCallArgs(0)
	if err != nil {
		t.Fatalf("Error getting args for call 0: %v", err)
	}
	compareIntPointerSlicesWithNil(t, []*int{intPtr(1), nil, intPtr(3)}, args0)

	// Call 1
	args1, err := mockStats.GetCallArgs(1)
	if err != nil {
		t.Fatalf("Error getting args for call 1: %v", err)
	}
	compareIntPointerSlicesWithNil(t, []*int{intPtr(4), nil, nil}, args1)

	// Call 2 (final flush)
	args2, err := mockStats.GetCallArgs(2)
	if err != nil {
		t.Fatalf("Error getting args for call 2: %v", err)
	}
	compareIntPointerSlicesWithNil(t, []*int{intPtr(7)}, args2)
}

// Helper function needed for the TestCollector_HandlesNilPointers test
func intPtr(v int) *int { return &v }
