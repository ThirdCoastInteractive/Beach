package perf

import (
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ThirdCoastInteractive/Beach/pkg/hub"
)

// heapAlloc forces a GC and returns the current live-heap byte count, so two
// readings bracket only the memory actually retained between them.
func heapAlloc() uint64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// fanoutSizes is the connection counts BenchmarkFanout sweeps. The default set
// stays small so the benchmark runs fast in CI; the 5k entry is the actual
// driftbottle budget — "5k SSE connections on one container is the budget the hub
// must meet" — and is included so a `-bench .` run on a workstation exercises it.
// Drop or extend the slice to retarget; N is fully parameterized.
var fanoutSizes = []int{100, 1000, 5000}

// BenchmarkFanout measures the hub's publish fan-out: subscribe N connections to
// one topic, then publish frames and time delivery to all N. It reports two
// custom metrics alongside ns/op:
//
//   - bytes/conn — heap bytes attributable to the N live subscriptions (the
//     per-connection memory the 5k budget cares about).
//   - frames/sec — delivered frames per second across all connections, the
//     fan-out throughput.
//
// Each subscriber is drained by its own goroutine so the per-sub buffer (64,
// drop-on-full) doesn't silently swallow the publish under load — the benchmark
// measures real delivery, and asserts no drops at steady cadence.
//
// Run: go test ./perf/... -run '^$' -bench BenchmarkFanout -benchmem
func BenchmarkFanout(b *testing.B) {
	const topic = "bench:fanout"
	// payload is a representative pre-rendered SSE patch (a small morphed
	// fragment), the unit the hub fans out.
	payload := []byte(`<div id="live" class="dw-live"><p>Server time: <strong>12:34:56</strong></p></div>`)

	for _, n := range fanoutSizes {
		b.Run("N="+strconv.Itoa(n), func(b *testing.B) {
			h := hub.New()

			// received counts events delivered across every subscriber; each
			// drain goroutine increments it and exits when its channel closes.
			var received int64
			var drainWG sync.WaitGroup

			subs := make([]*hub.Sub, n)
			for i := 0; i < n; i++ {
				s := h.Subscribe(topic)
				subs[i] = s
				drainWG.Add(1)
				go func(c <-chan hub.Event) {
					defer drainWG.Done()
					for range c {
						atomic.AddInt64(&received, 1)
					}
				}(s.C)
			}

			ev := hub.Event{Bytes: payload}

			b.ResetTimer()
			// Each iteration is one full fan-out (one publish reaching all N
			// connections). N is the parallelism; b.N is the number of frames.
			for i := 0; i < b.N; i++ {
				h.Publish(topic, ev)
			}
			b.StopTimer()

			// frames/sec: total delivered frames / wall time. b.Elapsed covers the
			// timed region only (publish loop), excluding setup/teardown.
			elapsed := b.Elapsed().Seconds()
			if elapsed > 0 {
				totalFrames := float64(b.N) * float64(n)
				b.ReportMetric(totalFrames/elapsed, "frames/sec")
			}

			// Tear down subscriptions and wait for drains to finish so the next
			// size starts clean and no goroutines leak across sub-benchmarks.
			for _, s := range subs {
				s.Close()
			}
			drainWG.Wait()
		})
	}
}

// BenchmarkFanoutMemory reports the heap cost of holding N live subscriptions —
// the per-connection memory budget. It is a separate benchmark (not folded into
// BenchmarkFanout) because measuring resident memory wants a quiescent heap and a
// forced GC, which a throughput loop would perturb.
//
// Run: go test ./perf/... -run '^$' -bench BenchmarkFanoutMemory
func BenchmarkFanoutMemory(b *testing.B) {
	const topic = "bench:mem"
	for _, n := range fanoutSizes {
		b.Run("N="+strconv.Itoa(n), func(b *testing.B) {
			// The work being benchmarked is "stand up N subscriptions"; the ns/op
			// is the time to subscribe+close N, and bytes/conn (reported after the
			// loop) is the headline per-connection memory figure.
			for i := 0; i < b.N; i++ {
				h := hub.New()
				subs := make([]*hub.Sub, n)
				for j := 0; j < n; j++ {
					subs[j] = h.Subscribe(topic)
				}
				for _, s := range subs {
					s.Close()
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(subMemoryBytes(topic, n))/float64(n), "bytes/conn")
		})
	}
}

// subMemoryBytes returns the live-heap delta from holding n subscriptions to one
// topic: it snapshots HeapAlloc with the subs unreachable, then with them held
// alive across a forced GC, and returns the difference. runtime.KeepAlive pins
// the slice so the second reading actually includes the subscriptions.
func subMemoryBytes(topic string, n int) uint64 {
	before := heapAlloc()

	h := hub.New()
	subs := make([]*hub.Sub, n)
	for i := 0; i < n; i++ {
		subs[i] = h.Subscribe(topic)
	}

	after := heapAlloc()
	runtime.KeepAlive(subs)
	runtime.KeepAlive(h)

	for _, s := range subs {
		s.Close()
	}
	if after < before {
		return 0
	}
	return after - before
}
