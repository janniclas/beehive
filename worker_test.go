package beehive

import (
	"fmt"
	"sync"
	"testing"
)

// Helper to collect ALL results from a channel until it's closed
func collectResults[E any](ch <-chan *E) []*E {
	var results []*E
	for res := range ch { // Use range loop which handles channel closing automatically
		if res != nil { // Good practice although worker shouldn't send nil on success
			results = append(results, res)
		}
	}
	return results
}

// Helper to collect ALL errors from a channel until it's closed
func collectErrors(errc <-chan error) []error {
	var errs []error
	for err := range errc { // Use range loop
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

func TestWorker_ProcessSingleItem(t *testing.T) {
	t.Parallel()
	in := make(chan *int)
	// Use a reasonable buffer, maybe matching potential number of outputs
	out := make(chan *string, 1)
	errc := make(chan error, 1)
	var wg sync.WaitGroup

	worker := Worker[int, string]{
		Work: func(i *int) (*string, error) {
			s := fmt.Sprintf("processed:%d", *i)
			return &s, nil
		},
	}

	wg.Add(1)
	go worker.Run(in, out, errc, &wg)

	val := 10
	in <- &val
	close(in) // Signal no more input

	// Wait for the worker goroutine to fully finish before proceeding
	wg.Wait()

	// Now that the worker is done, it won't send any more results or errors.
	// Close the output channels to signal the end of streams.
	close(out)
	close(errc)

	// Collect results and errors by ranging over the closed channels
	results := collectResults(out) // No stop signal needed
	errs := collectErrors(errc)    // No stop signal needed

	// --- Assertions ---
	if len(errs) != 0 {
		t.Errorf("Expected no errors, got %v", errs)
	}
	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}
	if *results[0] != "processed:10" {
		t.Errorf("Expected result 'processed:10', got '%s'", *results[0])
	}
}

// func TestWorker_ProcessMultipleItems(t *testing.T) {
// 	t.Parallel()
// 	in := make(chan *int)
// 	out := make(chan *string, 5)
// 	errc := make(chan error, 1)
// 	var wg sync.WaitGroup

// 	worker := Worker[int, string]{
// 		Do: func(i *int) (*string, error) {
// 			s := fmt.Sprintf("item:%d", *i)
// 			return &s, nil
// 		},
// 	}

// 	wg.Add(1)
// 	go worker.Run(in, out, errc, &wg)

// 	items := []int{1, 2, 3, 4, 5}
// 	go func() {
// 		for i := range items {
// 			// Need to capture loop variable correctly for pointer
// 			v := items[i]
// 			in <- &v
// 		}
// 		close(in)
// 	}()

// 	wg.Wait() // Wait for worker to finish processing all items
// 	close(out)
// 	close(errc)

// 	// Check results
// 	stopSignal := make(chan struct{})
// 	close(stopSignal)
// 	results := collectResults(out, stopSignal)
// 	errs := collectErrors(errc, stopSignal)

// 	if len(errs) != 0 {
// 		t.Errorf("Expected no errors, got %v", errs)
// 	}
// 	if len(results) != len(items) {
// 		t.Fatalf("Expected %d results, got %d", len(items), len(results))
// 	}
// 	// Basic check, order might vary with multiple workers, but here it's 1 worker
// 	expected := map[string]bool{"item:1": true, "item:2": true, "item:3": true, "item:4": true, "item:5": true}
// 	for _, res := range results {
// 		if !expected[*res] {
// 			t.Errorf("Unexpected result '%s'", *res)
// 		}
// 		delete(expected, *res) // Mark as found
// 	}
// 	if len(expected) > 0 {
// 		t.Errorf("Did not find all expected results, missing: %v", expected)
// 	}
// }

// func TestWorker_HandleError(t *testing.T) {
// 	t.Parallel()
// 	in := make(chan *int)
// 	out := make(chan *string, 1)
// 	errc := make(chan error, 1)
// 	var wg sync.WaitGroup
// 	testErr := errors.New("processing failed")

// 	worker := Worker[int, string]{
// 		Do: func(i *int) (*string, error) {
// 			if *i == 2 {
// 				return nil, testErr
// 			}
// 			s := fmt.Sprintf("val:%d", *i)
// 			return &s, nil
// 		},
// 	}

// 	wg.Add(1)
// 	go worker.Run(in, out, errc, &wg)

// 	val := 2
// 	in <- &val
// 	close(in)

// 	wg.Wait()
// 	close(out)
// 	close(errc)

// 	// Check results
// 	stopSignal := make(chan struct{})
// 	close(stopSignal)
// 	results := collectResults(out, stopSignal)
// 	errs := collectErrors(errc, stopSignal)

// 	if len(results) != 0 {
// 		t.Errorf("Expected no successful results, got %v", results)
// 	}
// 	if len(errs) != 1 {
// 		t.Fatalf("Expected 1 error, got %d: %v", len(errs), errs)
// 	}
// 	if !errors.Is(errs[0], testErr) {
// 		t.Errorf("Expected error '%v', got '%v'", testErr, errs[0])
// 	}
// }

// func TestWorker_ContinueAfterError(t *testing.T) {
// 	t.Parallel()
// 	in := make(chan *int)
// 	out := make(chan *string, 2)
// 	errc := make(chan error, 1)
// 	var wg sync.WaitGroup
// 	testErr := errors.New("failed on 2")

// 	worker := Worker[int, string]{
// 		Do: func(i *int) (*string, error) {
// 			if *i == 2 {
// 				return nil, testErr
// 			}
// 			s := fmt.Sprintf("ok:%d", *i)
// 			return &s, nil
// 		},
// 	}

// 	wg.Add(1)
// 	go worker.Run(in, out, errc, &wg)

// 	items := []int{1, 2, 3}
// 	go func() {
// 		for i := range items {
// 			v := items[i]
// 			in <- &v
// 		}
// 		close(in)
// 	}()

// 	wg.Wait()
// 	close(out)
// 	close(errc)

// 	// Check results
// 	stopSignal := make(chan struct{})
// 	close(stopSignal)
// 	results := collectResults(out, stopSignal)
// 	errs := collectErrors(errc, stopSignal)

// 	// Check errors
// 	if len(errs) != 1 {
// 		t.Fatalf("Expected 1 error, got %d: %v", len(errs), errs)
// 	}
// 	if !errors.Is(errs[0], testErr) {
// 		t.Errorf("Expected error '%v', got '%v'", testErr, errs[0])
// 	}

// 	// Check successful results
// 	if len(results) != 2 {
// 		t.Fatalf("Expected 2 successful results, got %d", len(results))
// 	}
// 	expected := map[string]bool{"ok:1": true, "ok:3": true}
// 	for _, res := range results {
// 		if !expected[*res] {
// 			t.Errorf("Unexpected result '%s'", *res)
// 		}
// 		delete(expected, *res)
// 	}
// 	if len(expected) > 0 {
// 		t.Errorf("Did not find all expected results, missing: %v", expected)
// 	}
// }

// func TestWorker_SignalsDoneOnClose(t *testing.T) {
// 	t.Parallel()
// 	in := make(chan *int)
// 	out := make(chan *string, 1)
// 	errc := make(chan error, 1)
// 	var wg sync.WaitGroup

// 	worker := Worker[int, string]{
// 		Do: func(i *int) (*string, error) { time.Sleep(5 * time.Millisecond); s := "done"; return &s, nil }, // Simulate some work
// 	}

// 	wg.Add(1)
// 	go worker.Run(in, out, errc, &wg)

// 	// Send nothing or maybe one item
// 	val := 1
// 	in <- &val
// 	close(in)

// 	// Wait with timeout
// 	waitChan := make(chan struct{})
// 	go func() {
// 		wg.Wait()
// 		close(waitChan)
// 	}()

// 	select {
// 	case <-waitChan:
// 		// Success
// 	case <-time.After(100 * time.Millisecond):
// 		t.Fatal("Worker did not signal Done within timeout")
// 	}
// 	// Clean up other channels only after confirming wg.Wait finished
// 	close(out)
// 	close(errc)
// }

// func TestWorker_DoNothingHelper(t *testing.T) {
// 	t.Parallel()
// 	in := make(chan *int)
// 	out := make(chan *int, 1) // Note: E is same as T here
// 	errc := make(chan error, 1)
// 	var wg sync.WaitGroup

// 	worker := Worker[int, int]{ // T and E are int
// 		Do: DoNothing[int],
// 	}

// 	wg.Add(1)
// 	go worker.Run(in, out, errc, &wg)

// 	val := 123
// 	in <- &val
// 	close(in)

// 	wg.Wait()
// 	close(out)
// 	close(errc)

// 	stopSignal := make(chan struct{})
// 	close(stopSignal)
// 	results := collectResults(out, stopSignal) // Type is []*int
// 	errs := collectErrors(errc, stopSignal)

// 	if len(errs) != 0 {
// 		t.Errorf("Expected no errors, got %v", errs)
// 	}
// 	if len(results) != 1 {
// 		t.Fatalf("Expected 1 result, got %d", len(results))
// 	}
// 	// Check if it's the same value
// 	if *results[0] != val {
// 		t.Errorf("Expected result %d, got %d", val, *results[0])
// 	}
// 	// Optionally check if it's the exact same pointer (depends on use case)
// 	// if results[0] != &val {
// 	//  t.Errorf("Expected same pointer %p, got %p", &val, results[0])
// 	// }
// }
