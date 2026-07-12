// Command askmuskie-load is a load generator for askmuskie's realtime pond. It
// opens N long-lived SSE connections to /pond/stream — each a distinct visitor
// (its own am_sid cookie) — drains the world-snapshot stream, and has a
// configurable fraction of visitors periodically ask for wisdom and drop food.
// It reports, once a second, how many connections are held, the frame and byte
// throughput received, errors, and the inter-arrival latency of the "hot" frame
// (sampled from a subset of clients).
//
// It is the "can it take a couple thousand?" tool. Run the server, then:
//
//	go run ./cmd/askmuskie-load -url http://localhost:8099 -n 2000 -ramp 20s -dur 60s -ask 0.02 -food 0.05
package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"math"
	mrand "math/rand"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	url := flag.String("url", "http://localhost:8099", "base URL of the running askmuskie server")
	n := flag.Int("n", 500, "number of concurrent visitors to simulate")
	ramp := flag.Duration("ramp", 15*time.Second, "spread connection starts over this duration")
	dur := flag.Duration("dur", 60*time.Second, "hold the load this long after ramp-up")
	askProb := flag.Float64("ask", 0.02, "per-visitor-per-second probability of asking")
	foodProb := flag.Float64("food", 0.05, "per-visitor-per-second probability of dropping food")
	sampleN := flag.Int("sample", 25, "measure hot-frame latency on this many visitors")
	flag.Parse()

	base := strings.TrimRight(*url, "/")
	streamURL := base + "/pond/stream"

	// Tuned transport: many long-lived connections to one host.
	tr := &http.Transport{
		MaxConnsPerHost:     0,
		MaxIdleConns:        0,
		MaxIdleConnsPerHost: *n + 16,
		DisableCompression:  true,
	}
	client := &http.Client{Transport: tr}

	var (
		connected atomic.Int64
		frames    atomic.Int64
		bytesRecv atomic.Int64
		errs      atomic.Int64
		lat       = &latColl{}
	)

	sids := make([]string, *n)
	for i := range sids {
		sids[i] = newID()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Reporter.
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		start := time.Now()
		var lastFrames, lastBytes int64
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				f, b := frames.Load(), bytesRecv.Load()
				df, db := f-lastFrames, b-lastBytes
				lastFrames, lastBytes = f, b
				p50, p99 := lat.percentiles()
				fmt.Printf("[%4ds] conns=%-5d frames/s=%-6d recv=%6.2f MB/s errs=%-4d hot p50=%5.1fms p99=%6.1fms\n",
					int(time.Since(start).Seconds()), connected.Load(), df,
					float64(db)/1e6, errs.Load(), p50, p99)
			}
		}
	}()

	// Spawn visitors, staggered across the ramp.
	var wg sync.WaitGroup
	gap := time.Duration(0)
	if *n > 0 {
		gap = *ramp / time.Duration(*n)
	}
	for i := 0; i < *n; i++ {
		select {
		case <-ctx.Done():
		default:
		}
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			runVisitor(ctx, client, streamURL, sids[idx], idx < *sampleN, &connected, &frames, &bytesRecv, &errs, lat)
		}(i)
		if gap > 0 {
			time.Sleep(gap)
		}
	}
	fmt.Printf("ramp complete: %d visitors launched; holding for %s\n", *n, *dur)

	// Action driver: each second, each visitor asks/feeds with the given odds.
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				for _, sid := range sids {
					if mrand.Float64() < *askProb {
						go postCmd(client, base+"/pond/ask", sid, `{"skip":false}`)
					}
					if mrand.Float64() < *foodProb {
						x := (mrand.Float64()*2 - 1) * 3
						z := (mrand.Float64()*2 - 1) * 1.5
						go postCmd(client, base+"/pond/food", sid, fmt.Sprintf(`{"x":%.2f,"z":%.2f}`, x, z))
					}
				}
			}
		}
	}()

	time.Sleep(*dur)
	cancel()
	wg.Wait()

	p50, p99 := lat.percentiles()
	fmt.Printf("\nDONE: peak conns observed via reporter; total frames=%d, total recv=%.1f MB, errors=%d, final hot p50=%.1fms p99=%.1fms\n",
		frames.Load(), float64(bytesRecv.Load())/1e6, errs.Load(), p50, p99)
}

// runVisitor holds one SSE connection open and drains it until ctx is done.
func runVisitor(ctx context.Context, client *http.Client, streamURL, sid string, sampled bool,
	connected, frames, bytesRecv, errs *atomic.Int64, lat *latColl) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if err != nil {
		errs.Add(1)
		return
	}
	req.Header.Set("Accept", "text/event-stream")
	req.AddCookie(&http.Cookie{Name: "am_sid", Value: sid})

	resp, err := client.Do(req)
	if err != nil {
		errs.Add(1)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		errs.Add(1)
		return
	}
	connected.Add(1)
	defer connected.Add(-1)

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var lastHot time.Time
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) < 6 || string(line[:5]) != "data:" {
			continue // SSE comment / blank separator
		}
		frames.Add(1)
		bytesRecv.Add(int64(len(line)) + 2)
		if sampled && containsHot(line) {
			now := time.Now()
			if !lastHot.IsZero() {
				lat.add(float64(now.Sub(lastHot).Microseconds()) / 1000.0)
			}
			lastHot = now
		}
	}
	if err := sc.Err(); err != nil && ctx.Err() == nil {
		errs.Add(1)
	}
}

// containsHot reports whether an SSE data line is a hot frame.
func containsHot(line []byte) bool { return strings.Contains(string(line), `"k":"hot"`) }

func postCmd(client *http.Client, url, sid, body string) {
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "am_sid", Value: sid})
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

// latColl collects hot-frame inter-arrival samples (ms) for percentile reports.
type latColl struct {
	mu sync.Mutex
	v  []float64
}

func (l *latColl) add(ms float64) {
	l.mu.Lock()
	if len(l.v) < 200000 {
		l.v = append(l.v, ms)
	}
	l.mu.Unlock()
}

// percentiles returns p50/p99 over the samples collected since the last call,
// then clears them so each report reflects the most recent window.
func (l *latColl) percentiles() (p50, p99 float64) {
	l.mu.Lock()
	v := l.v
	l.v = nil
	l.mu.Unlock()
	if len(v) == 0 {
		return 0, 0
	}
	sort.Float64s(v)
	return v[int(math.Min(float64(len(v)-1), float64(len(v))*0.50))],
		v[int(math.Min(float64(len(v)-1), float64(len(v))*0.99))]
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
