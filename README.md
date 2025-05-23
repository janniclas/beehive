[![Go Report Card](https://goreportcard.com/badge/github.com/janniclas/beehive)](https://goreportcard.com/report/github.com/janniclas/beehive)[![License](https://img.shields.io/badge/License-MIT%20-yellow.svg)](https://github.com/janniclas/beehive/blob/main/LICENSE)


![beehive logo](img/logo/png/gopher-beehive-txt_transparent.png)

## About
Beehive is a zero dependency, channel-based implementation of the Fan-Out and Fan-In pattern. It supports functionality to spawn a customizable number of generic worker goroutines. The worker must be configured with a custom function to process the incoming data.
Every worker is linked to a buffered collector that processes the incoming data.
The coordination is handled centrally by the dispatcher.


![Fan-Out and Fan-In](img/fan-out-fan-in.png)
*Illustration of the Fan-Out and Fan-In Pattern*
## Usage

```Go
// initialize worker that processes ints and produces strings
worker := beehive.Worker[int, string] {
	Work: func(i *int) (*string, error) {
		s := fmt.Sprintf("processed:%d", *i)
		return &s, nil
	},
}

buffer := 1
collect := func(s []*string) error {
	// perform action on all collected strings
	// e.g., store them to a database

	return nil
}
// initialize collector with matching type
collector := beehive.NewBufferedCollector[string](
	collect,
	beehive.BufferedCollectorConfig{
		BufferSize: &buffer,
	},
)

producer := []int{1, 2, 3, 4}

// configure the number of workers. depending on the
// task the number of cores might be a good idea
numWorker := 4

dispatcher := beehive.NewDispatcher(
	worker,
	slices.Values(producer),
	collector,
	beehive.DispatcherConfig{
		NumWorker: &numWorker,
	},
)

// starts the processing and blocks until producer
// is finished and all workers finished processing
errs := dispatcher.Dispatch()
```
For more examples see the project's tests (take those with a grain of salt. We've got 100% test coverage, however most of the tests have been generated by AI).

### Configuration
beehive has been developed as a general purpose solution with very few configuration options.

#### Rate limit
The rate limit is applied in the dispatcher before forwarding the tasks to the workers. A rate limit of, e.g., 5 seconds ensures that only every 5 seconds a task is worked on.
A rate limit is often used to limit the amount of requests to an external API.

#### Number of workers
Defines how many workers are started from the dispatcher. Each worker runs in its own goroutine. The optimal number of workers highly depends on your use case. It defaults to the number of CPU cores.
