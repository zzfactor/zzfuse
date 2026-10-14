package event

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// benchmarkHandler is a simple handler for benchmarking
type benchmarkHandler struct {
}

func (h *benchmarkHandler) HandleEvent(ctx context.Context, event Event) error {
	// Simulate some work
	time.Sleep(1 * time.Microsecond)
	return nil
}

// BenchmarkEventBusPublish benchmarks the original event bus
func BenchmarkInMemoryEventBusPublish(b *testing.B) {
	bus := NewInMemoryEventBus(1000, 10)
	defer func() {
		if err := bus.Close(); err != nil {
			b.Errorf("failed to close bus: %v", err)
		}
	}()

	handler := &benchmarkHandler{}
	bus.RegisterHandler(TestEventTypeA, handler)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := bus.Publish(context.Background(), Event{
				Type:      TestEventTypeA,
				Timestamp: time.Now(),
				Schema:    "benchmark",
				Data:      "benchmark data",
			}); err != nil {
				b.Errorf("failed to publish event: %v", err)
			}
		}
	})
}

// BenchmarkEventBusWithPoolPublish benchmarks the worker pool event bus
func BenchmarkPooledEventBusPublish(b *testing.B) {
	bus := NewPooledEventBus(10)
	defer func() {
		if err := bus.Close(); err != nil {
			b.Errorf("failed to close bus: %v", err)
		}
	}()

	handler := &benchmarkHandler{}
	bus.RegisterHandler(TestEventTypeA, handler)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := bus.Publish(context.Background(), Event{
				Type:      TestEventTypeA,
				Timestamp: time.Now(),
				Schema:    "benchmark",
				Data:      "benchmark data",
			}); err != nil {
				b.Errorf("failed to publish event: %v", err)
			}
		}
	})
}

// BenchmarkEventBusMultipleHandlers benchmarks with multiple handlers
func BenchmarkInMemoryEventBusMultipleHandlers(b *testing.B) {
	bus := NewInMemoryEventBus(1000, 10)
	defer func() {
		if err := bus.Close(); err != nil {
			b.Errorf("failed to close bus: %v", err)
		}
	}()

	// Register multiple handlers
	for i := 0; i < 5; i++ {
		handler := &benchmarkHandler{}
		bus.RegisterHandler(TestEventTypeA, handler)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := bus.Publish(context.Background(), Event{
				Type:      TestEventTypeA,
				Timestamp: time.Now(),
				Schema:    "benchmark",
				Data:      "benchmark data",
			}); err != nil {
				b.Errorf("failed to publish event: %v", err)
			}
		}
	})
}

// BenchmarkEventBusWithPoolMultipleHandlers benchmarks worker pool with multiple handlers
func BenchmarkPooledEventBusMultipleHandlers(b *testing.B) {
	bus := NewPooledEventBus(10)
	defer func() {
		if err := bus.Close(); err != nil {
			b.Errorf("failed to close bus: %v", err)
		}
	}()

	// Register multiple handlers
	for i := 0; i < 5; i++ {
		handler := &benchmarkHandler{}
		bus.RegisterHandler(TestEventTypeA, handler)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := bus.Publish(context.Background(), Event{
				Type:      TestEventTypeA,
				Timestamp: time.Now(),
				Schema:    "benchmark",
				Data:      "benchmark data",
			}); err != nil {
				b.Errorf("failed to publish event: %v", err)
			}
		}
	})
}

// BenchmarkEventBusBatchPublish benchmarks batch publishing
func BenchmarkEventBusBatchPublish(b *testing.B) {
	bus := NewPooledEventBus(10)
	defer func() {
		if err := bus.Close(); err != nil {
			b.Errorf("failed to close bus: %v", err)
		}
	}()

	handler := &benchmarkHandler{}
	bus.RegisterHandler(TestEventTypeA, handler)

	// Prepare batch of events
	events := make([]Event, 100)
	for i := range events {
		events[i] = Event{
			Type:      TestEventTypeA,
			Timestamp: time.Now(),
			Schema:    "benchmark",
			Data:      "benchmark data",
		}
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := bus.PublishBatch(context.Background(), events); err != nil {
				b.Errorf("failed to publish batch: %v", err)
			}
		}
	})
}

// BenchmarkMemoryAllocation benchmarks memory allocation patterns
func BenchmarkMemoryAllocation(b *testing.B) {
	bus := NewPooledEventBus(10)
	defer func() {
		if err := bus.Close(); err != nil {
			b.Errorf("failed to close bus: %v", err)
		}
	}()

	handler := &benchmarkHandler{}
	bus.RegisterHandler(TestEventTypeA, handler)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if err := bus.Publish(context.Background(), Event{
			Type:      TestEventTypeA,
			Timestamp: time.Now(),
			Schema:    "benchmark",
			Data:      "benchmark data",
		}); err != nil {
			b.Errorf("failed to publish event: %v", err)
		}
	}
}

// BenchmarkConcurrentPublish benchmarks concurrent publishing
func BenchmarkConcurrentPublish(b *testing.B) {
	bus := NewPooledEventBus(10)
	defer func() {
		if err := bus.Close(); err != nil {
			b.Errorf("failed to close bus: %v", err)
		}
	}()

	handler := &benchmarkHandler{}
	bus.RegisterHandler(TestEventTypeA, handler)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := bus.Publish(context.Background(), Event{
				Type:      TestEventTypeA,
				Timestamp: time.Now(),
				Schema:    "benchmark",
				Data:      "benchmark data",
			}); err != nil {
				b.Errorf("failed to publish event: %v", err)
			}
		}
	})
}

// TestWorkerPoolPerformance demonstrates the performance improvement
func TestWorkerPoolPerformance(t *testing.T) {
	// Test with different worker pool sizes
	workerCounts := []int{1, 5, 10, 20}

	for _, workers := range workerCounts {
		t.Run(fmt.Sprintf("Workers_%d", workers), func(t *testing.T) {
			bus := NewPooledEventBus(workers)
			defer func() {
				if err := bus.Close(); err != nil {
					t.Errorf("failed to close bus: %v", err)
				}
			}()

			handler := &benchmarkHandler{}
			bus.RegisterHandler(TestEventTypeA, handler)

			// Publish events and measure time
			start := time.Now()
			for i := 0; i < 1000; i++ {
				if err := bus.Publish(context.Background(), Event{
					Type:      TestEventTypeA,
					Timestamp: time.Now(),
					Schema:    "test",
					Data:      "test data",
				}); err != nil {
					t.Errorf("failed to publish event: %v", err)
				}
			}
			duration := time.Since(start)

			// Get stats
			stats := bus.GetStats()

			t.Logf("Workers: %d, Duration: %v, Queue Size: %d/%d",
				workers, duration, stats.WorkerPool.QueueSize, stats.WorkerPool.QueueCapacity)
		})
	}
}

// TestWorkerPoolStats verifies that stats are working correctly
func TestWorkerPoolStats(t *testing.T) {
	bus := NewPooledEventBus(5)
	defer func() {
		if err := bus.Close(); err != nil {
			t.Errorf("failed to close bus: %v", err)
		}
	}()

	// Register handlers
	for i := 0; i < 3; i++ {
		handler := &benchmarkHandler{}
		bus.RegisterHandler(TestEventTypeA, handler)
	}

	// Get initial stats
	stats := bus.GetStats()
	if stats.TotalHandlers != 3 {
		t.Errorf("Expected 3 handlers, got %d", stats.TotalHandlers)
	}
	if stats.WorkerPool.Workers != 5 {
		t.Errorf("Expected 5 workers, got %d", stats.WorkerPool.Workers)
	}

	// Publish some events
	for i := 0; i < 10; i++ {
		if err := bus.Publish(context.Background(), Event{
			Type:      TestEventTypeA,
			Timestamp: time.Now(),
			Schema:    "test",
			Data:      "test data",
		}); err != nil {
			t.Errorf("failed to publish event: %v", err)
		}
	}

	// Give some time for processing
	time.Sleep(10 * time.Millisecond)

	// Get stats again
	stats = bus.GetStats()
	t.Logf("Stats after publishing: %+v", stats)
}
