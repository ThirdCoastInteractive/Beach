package pond

import (
	"context"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ThirdCoastInteractive/Beach/pkg/hub"

	"github.com/ThirdCoastInteractive/Beach/cmd/askmuskie/internal/quotes"
	"github.com/ThirdCoastInteractive/Beach/cmd/askmuskie/internal/speech"
)

func newTestPond(t *testing.T) *Pond {
	t.Helper()
	g, err := speech.New(strings.NewReader("hi HH AY\nthere DH EH R"))
	if err != nil {
		t.Fatalf("speech.New: %v", err)
	}
	w := quotes.NewSet([]quotes.Quote{{Text: "Hi there.", Attribution: "The Muskie"}})
	return New(hub.New(), w, g, nil, nil)
}

// tjoin is an anonymous-visitor join helper for tests.
func tjoin(p *Pond, sid string) { p.join(sid, 0, "", "", -1, -1) }

func TestPresenceConnCounting(t *testing.T) {
	p := newTestPond(t)
	tjoin(p, "a")
	tjoin(p, "a") // second tab
	if p.users["a"].conns != 2 {
		t.Fatalf("conns = %d, want 2", p.users["a"].conns)
	}
	p.leave("a")
	if _, ok := p.users["a"]; !ok {
		t.Fatal("visitor removed while a tab is still open")
	}
	p.leave("a")
	if _, ok := p.users["a"]; ok {
		t.Fatal("visitor still present after last tab closed")
	}
}

func TestAskQueueFIFO(t *testing.T) {
	p := newTestPond(t)
	tjoin(p, "a")
	tjoin(p, "b")
	p.ask("a", false, "")
	p.ask("b", false, "")
	if got, _ := p.front(); got != "a" {
		t.Fatalf("front = %q, want a (FIFO)", got)
	}
	p.ask("a", false, "") // duplicate ask is ignored
	if len(p.queue) != 2 {
		t.Fatalf("queue len = %d, want 2 (no duplicate)", len(p.queue))
	}
}

func TestPayToSkip(t *testing.T) {
	p := newTestPond(t)
	tjoin(p, "a")
	tjoin(p, "b")
	p.ask("a", false, "") // plain
	p.ask("b", true, "")  // spends food for priority
	if got, _ := p.front(); got != "b" {
		t.Fatalf("front = %q, want b (paid priority)", got)
	}
	if p.users["b"].food != startFood-1 {
		t.Fatalf("b food = %d, want %d (one spent)", p.users["b"].food, startFood-1)
	}
}

func TestFoodAccrualAndCap(t *testing.T) {
	p := newTestPond(t)
	tjoin(p, "a")
	p.users["a"].food = 0
	p.accrueFood(accrualSecs * 3.5) // 3 whole units
	if p.users["a"].food != 3 {
		t.Fatalf("food = %d, want 3 after %.0fs", p.users["a"].food, accrualSecs*3.5)
	}
	p.users["a"].food = foodCap
	p.accrueFood(accrualSecs * 5)
	if p.users["a"].food != foodCap {
		t.Fatalf("food = %d, want cap %d", p.users["a"].food, foodCap)
	}
}

func TestDropFoodCost(t *testing.T) {
	p := newTestPond(t)
	tjoin(p, "a")
	p.dropFood("a", 1, 1)
	if len(p.food) != 1 || p.users["a"].food != startFood-1 {
		t.Fatalf("after drop: pellets=%d food=%d", len(p.food), p.users["a"].food)
	}
	p.users["a"].food = 0
	p.dropFood("a", 2, 2)
	if len(p.food) != 1 {
		t.Fatal("dropped a pellet with no food")
	}
}

func TestDropFoodStaysInTank(t *testing.T) {
	p := newTestPond(t)
	tjoin(p, "a")
	p.users["a"].food = 5
	p.dropFood("a", 100, -100) // way outside the tank
	f := p.food[len(p.food)-1]
	if f.x > baseHalfW-margin+0.001 || f.z < -(baseHalfD-margin)-0.001 {
		t.Fatalf("pellet (%.2f,%.2f) escaped the tank", f.x, f.z)
	}
	if f.y != foodY {
		t.Fatalf("pellet y=%.2f, want surface %.2f", f.y, foodY)
	}
}

func TestTankGrowsWithCount(t *testing.T) {
	p := newTestPond(t)
	if w, _ := p.tankTarget(); w != baseHalfW {
		t.Fatalf("empty tank target w=%.2f, want base %.2f", w, baseHalfW)
	}
	for i := 0; i < 72; i++ { // sqrt(72/8) = 3 = maxScale
		tjoin(p, "u"+strconv.Itoa(i))
	}
	if p.countConnected() != 72 {
		t.Fatalf("count = %d, want 72", p.countConnected())
	}
	w, d := p.tankTarget()
	if math.Abs(w-baseHalfW*maxScale) > 1e-9 || math.Abs(d-baseHalfD*maxScale) > 1e-9 {
		t.Fatalf("72 visitors: target (%.2f,%.2f), want (%.2f,%.2f)", w, d, baseHalfW*maxScale, baseHalfD*maxScale)
	}
}

func TestEatingFeedsAndCheers(t *testing.T) {
	p := newTestPond(t)
	tjoin(p, "a")
	p.users["a"].food = 5
	p.hunger = 80
	p.happy = 50
	p.dropFood("a", 1, 0)
	id := p.food[0].id
	p.eat(id)
	if p.hunger != 80-eatHunger {
		t.Fatalf("hunger = %.0f, want %d", p.hunger, 80-eatHunger)
	}
	if p.happy != 50+eatHappy {
		t.Fatalf("happy = %.0f, want %d", p.happy, 50+eatHappy)
	}
	if len(p.food) != 0 {
		t.Fatal("pellet not consumed")
	}
}

func TestSleepsWhenTiredAndWakesRested(t *testing.T) {
	p := newTestPond(t)
	p.energy = sleepEnter - 1
	base := time.Unix(0, 0)
	p.step(0.05, base) // roam -> sleep (too tired)
	if p.state != "sleep" {
		t.Fatalf("state = %q, want sleep", p.state)
	}
	// Nap until rested; energy recovers while sleeping.
	for i := 0; i < 5000 && p.state == "sleep"; i++ {
		p.step(0.05, base)
	}
	if p.state != "roam" || p.energy < sleepExit {
		t.Fatalf("after nap: state=%q energy=%.0f", p.state, p.energy)
	}
}

func TestSpeakFlowServesAndReleases(t *testing.T) {
	p := newTestPond(t)
	tjoin(p, "a")
	p.ask("a", false, "")

	base := time.Unix(0, 0)
	// Run the FSM until he's speaking (roam->approach->speak).
	reached := false
	for i := 0; i < 400; i++ {
		p.step(0.05, base)
		if p.state == "speak" {
			reached = true
			break
		}
	}
	if !reached {
		t.Fatalf("never reached speak (state=%q)", p.state)
	}
	if p.serving != "a" || p.utter != 1 || p.quoteText == "" || p.quotePhon == "" {
		t.Fatalf("speak state wrong: serving=%q utter=%d text=%q phon=%q", p.serving, p.utter, p.quoteText, p.quotePhon)
	}
	if len(p.queue) != 0 {
		t.Fatalf("queue should be empty while serving, got %d", len(p.queue))
	}
	// Advance well past the utterance; he returns to roaming.
	p.step(0.05, base.Add(time.Hour))
	if p.state != "roam" || p.serving != "" {
		t.Fatalf("after speaking: state=%q serving=%q", p.state, p.serving)
	}
}

func TestQueuePreemptsGrazing(t *testing.T) {
	p := newTestPond(t)
	tjoin(p, "a")
	tjoin(p, "b")
	p.users["a"].food = 5
	p.dropFood("a", 3, 0)

	base := time.Unix(0, 0)
	p.step(0.05, base) // roam -> graze (food present, no queue)
	if p.state != "graze" {
		t.Fatalf("state = %q, want graze", p.state)
	}
	p.ask("b", false, "")  // a hungry visitor arrives
	p.step(0.05, base) // queue must preempt the graze
	if p.state != "approach" || p.serving != "b" {
		t.Fatalf("queue did not preempt graze: state=%q serving=%q", p.state, p.serving)
	}
}

func TestPickQuoteUsesOracleResultElseFlatFile(t *testing.T) {
	p := newTestPond(t)
	p.genResult = &oracleResult{ok: true, utt: Utterance{Text: "The current decides, not the fish.", Voice: "doom"}}
	if text, attr, voice := p.pickQuote(); text != "The current decides, not the fish." || attr != "The Muskie" || voice != "doom" {
		t.Fatalf("pickQuote = (%q,%q,%q), want the oracle line + voice", text, attr, voice)
	}
	// A failed/empty result, or none, falls back to the flat-file quote (default voice).
	p.genResult = &oracleResult{ok: false}
	if text, _, voice := p.pickQuote(); text != "Hi there." || voice != "" {
		t.Fatalf("failed result: pickQuote = (%q,%q), want flat-file fallback + default voice", text, voice)
	}
	p.genResult = nil
	if text, _, _ := p.pickQuote(); text != "Hi there." {
		t.Fatalf("nil result: pickQuote = %q, want flat-file fallback", text)
	}
}

func TestSpeakReadyWithoutOracle(t *testing.T) {
	if p := newTestPond(t); !p.speakReady() {
		t.Fatal("speakReady should be true when no oracle is configured")
	}
}

func TestAskSanitizesMessage(t *testing.T) {
	p := newTestPond(t)
	tjoin(p, "a")
	p.ask("a", false, "  hello\nthere\tfriend  ")
	if got := p.users["a"].askMsg; got != "hello there friend" {
		t.Fatalf("askMsg = %q, want flattened/trimmed", got)
	}
}

// fakeOracle voices a fixed line for the integration test.
type fakeOracle struct{ text string }

func (f fakeOracle) Speak(_ context.Context, _ SpeakRequest) (Utterance, bool) {
	return Utterance{Text: f.text, Voice: "deep"}, true
}

func TestOracleLineSpokenThroughRun(t *testing.T) {
	g, err := speech.New(strings.NewReader("hi HH AY"))
	if err != nil {
		t.Fatalf("speech.New: %v", err)
	}
	w := quotes.NewSet([]quotes.Quote{{Text: "fallback.", Attribution: "x"}})
	const line = "The hook waits in still water."
	p := New(hub.New(), w, g, nil, fakeOracle{text: line})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Run(ctx)

	p.Join("a", 0, "", "", -1, -1)
	p.Ask("a", false, "hello fish")

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got := make(chan string, 1)
		p.cmds <- func() { got <- p.quoteText } // read on the sim goroutine
		if <-got == line {
			return // the async oracle line made it all the way to speech
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("oracle line was never spoken through Run")
}

func TestLeaveWhileServedResets(t *testing.T) {
	p := newTestPond(t)
	tjoin(p, "a")
	p.ask("a", false, "")
	base := time.Unix(0, 0)
	for i := 0; i < 400 && p.serving != "a"; i++ {
		p.step(0.05, base)
	}
	if p.serving != "a" {
		t.Fatal("never served a")
	}
	p.leave("a")
	if p.serving != "" || p.state != "roam" {
		t.Fatalf("leaving the served visitor left state=%q serving=%q", p.state, p.serving)
	}
}
