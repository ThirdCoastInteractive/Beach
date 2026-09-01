// Package pond is askmuskie's authoritative, in-memory multiplayer sim: the
// Muskie in a glass tank, the visitors walking around it, the speak queue, the
// food, and the Muskie's living stats. A single goroutine owns all of it (Run),
// so there are no locks — public methods hand a closure to the loop.
//
// The world streams as frames over SSE (each tagged with a "k"):
//   - hot   (topic Topic, ~10Hz): the Muskie + stats + current quote.
//   - cold  (topic ColdTopic, ~1.5Hz): tank size, count, a sampled roster (with
//     each visitor's appearance), food, queue.
//   - moves (topic MovesTopic, ~6Hz): the sampled visitors' floor positions
//     (short-id keyed) so they animate smoothly as they walk.
//   - self  (topic SelfTopic(sid), ~1.5Hz): the visitor's own food, position,
//     appearance, and queue slot.
//
// Logged-in visitors carry an accountID; the Muskie attributes wisdom (when it
// speaks to them) and food (when they drop a pellet) to that account via a
// nil-safe Recorder, so stats persist without the sim ever touching Postgres.
package pond

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/ThirdCoastInteractive/Beach/pkg/hub"

	"github.com/ThirdCoastInteractive/Beach/cmd/askmuskie/internal/quotes"
	"github.com/ThirdCoastInteractive/Beach/cmd/askmuskie/internal/speech"
)

// Hub topics.
const (
	Topic      = "pond"
	ColdTopic  = "pond:cold"
	MovesTopic = "pond:moves"
)

// SelfTopic is the per-visitor topic for their own self frame.
func SelfTopic(sid string) string { return "self:" + sid }

// Recorder persists per-account stat increments off the hot path. Methods must
// be safe to call from the sim goroutine (non-blocking). A nil Recorder is fine.
type Recorder interface {
	Wisdom(accountID int64)
	Fed(accountID int64)
}

// SpeakRequest is the trusted live state (plus an untrusted visitor Message)
// handed to the Oracle to voice one aphorism. Snapshotted on the sim goroutine.
type SpeakRequest struct {
	Crowd     int
	Hunger    int
	Energy    int
	Happy     int
	State     string
	AccountID int64
	Name      string
	Message   string // untrusted visitor text ("" = unprompted)
}

// Utterance is what the Oracle produces: the aphorism plus a chosen voice name
// (empty = the client's default voice).
type Utterance struct {
	Text  string
	Voice string
}

// Oracle voices the Muskie from live state. Speak is called OFF the sim goroutine
// (it may block on the network) and returns the utterance + true, or a zero
// Utterance + false to fall back to a flat-file quote. A nil Oracle disables LLM
// voicing entirely.
type Oracle interface {
	Speak(ctx context.Context, req SpeakRequest) (Utterance, bool)
}

// Tank geometry. Base dims grow with the crowd (tankTarget); live dims are
// p.curHalfW / p.curHalfD.
const (
	baseHalfW = 4.5
	baseHalfD = 2.2
	waterTop  = 1.5
	waterBot  = -1.5
	margin    = 0.7
	ringGap   = 2.2
	roomGap   = 4.0 // how far past the visitor ring the floor extends (movement bound)
	foodY     = waterTop - 0.2
	restY     = waterBot + 0.4
	refUsers  = 8.0
	maxScale  = 3.0
	tankEase  = 0.5
)

// Appearance palette sizes — the client's body/skin palettes MUST match these
// lengths (static/js/avatar.js). Anon/unset visitors get an index from their id.
const (
	bodyPaletteLen = 14
	skinPaletteLen = 9
)

// Pacing.
const (
	tickHz      = 20
	hotEvery    = 2  // ~10Hz
	movesEvery  = 3  // ~6.7Hz
	coldEvery   = 13 // ~1.5Hz
	roamSpeed   = 2.0
	travelSpeed = 4.4
	grazeSpeed  = 4.0
	sleepSpeed  = 1.0
	arriveEps   = 0.45
	yawRate     = 6.0
	goldenAngle = 2.399963229728653
	cmdBuffer   = 2048
	speakBaseMs = 800
	speakPerTok = 120
	speakMaxMs  = 13000
	sampleCap   = 100
	queueCap    = 100
)

// Oracle pacing. oracleTimeout bounds the off-goroutine LLM call; oracleGraceMs
// is how long the Muskie hovers at the glass waiting for the line before falling
// back to a flat-file quote. The grace must exceed a warm generation (~5s for a
// short aphorism on CPU); cold model loads are avoided by keeping the sidecar
// model resident (OLLAMA_KEEP_ALIVE=-1) and pre-warming it.
const (
	oracleTimeout = 30 * time.Second
	oracleGraceMs = 10000 * time.Millisecond
	askMsgMax     = 200 // visitor message cap (bytes)
)

// Food economy.
const (
	accrualSecs = 10.0
	startFood   = 3
	foodCap     = 20
	maxPellets  = 40
)

// Living stats, 0..100; rates are points/sec.
const (
	hungerRise   = 100.0 / 200.0
	energyDrain  = 100.0 / 300.0
	sleepRecover = 100.0 / 45.0
	happyDecay   = 100.0 / 600.0
	eatHunger    = 28
	eatHappy     = 8
	askHappy     = 6
	starveHappy  = 6.0 / 60.0
	sleepEnter   = 14
	sleepExit    = 80
	startHunger  = 15
	startEnergy  = 100
	startHappy   = 70
)

type vec3 struct{ X, Y, Z float64 }

// user is one visitor. pos is their avatar's floor position (they walk around);
// short is a small id used in the compact moves frame; accountID is 0 for anon.
type user struct {
	conns     int
	accountID int64
	name      string
	short     uint32
	pos       vec3
	shape     string // appearance: shape key ("" = client default)
	bc        int    // body-color palette index
	sk        int    // skin palette index
	food      int
	accrual   float64
	queued    bool
	priority  int
	seq       int
	askMsg    string // untrusted message attached to their pending ask
}

type pellet struct {
	id      int
	x, y, z float64
}

// oracleResult is one async generation's outcome, tagged with the generation id
// it belongs to so stale results (a superseded ask) are dropped.
type oracleResult struct {
	gen int
	utt Utterance
	ok  bool
}

// Pond is the sim. Construct with New, start with Run, drive with the public
// command methods; never touch fields from outside.
type Pond struct {
	cmds   chan func()
	hub    *hub.Hub
	wisdom *quotes.Set
	voice  *speech.G2P
	rec    Recorder

	oracle     Oracle
	oracleCh   chan oracleResult
	genSeq     int           // bumped each approach; the in-flight generation id
	genResult  *oracleResult // the current generation's result, once it lands
	awaiting   bool          // hovering at the glass waiting for the oracle line
	awaitUntil time.Time

	t        float64
	tickN    int
	curHalfW float64
	curHalfD float64

	pos        vec3
	yaw        float64
	state      string // roam | approach | speak | graze | sleep
	target     vec3
	serving    string
	grazeID    int
	utter      int
	quoteText  string
	quoteAttr  string
	quoteVoice string
	quotePhon  string
	speakUntil time.Time

	hunger float64
	energy float64
	happy  float64

	users      map[string]*user
	queue      []string
	food       []pellet
	nextPellet int
	nextSeq    int
	nextShort  uint32
	joinCount  int
}

// New builds a Pond wired to the hub, its quote + phoneme sources, an optional
// stat recorder (nil disables per-account stats), and an optional Oracle (nil
// keeps the Muskie on flat-file quotes).
func New(h *hub.Hub, wisdom *quotes.Set, voice *speech.G2P, rec Recorder, orc Oracle) *Pond {
	return &Pond{
		cmds:     make(chan func(), cmdBuffer),
		hub:      h,
		wisdom:   wisdom,
		voice:    voice,
		rec:      rec,
		oracle:   orc,
		oracleCh: make(chan oracleResult, 16),
		state:    "roam",
		pos:      vec3{0, 0, 0},
		curHalfW: baseHalfW,
		curHalfD: baseHalfD,
		hunger:   startHunger,
		energy:   startEnergy,
		happy:    startHappy,
		users:    map[string]*user{},
	}
}

// Run owns the sim until ctx is cancelled. Call it in a goroutine.
func (p *Pond) Run(ctx context.Context) {
	tick := time.NewTicker(time.Second / tickHz)
	defer tick.Stop()
	last := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case fn := <-p.cmds:
			fn()
		case r := <-p.oracleCh:
			// Keep only the current generation's result; stale ones are dropped.
			if r.gen == p.genSeq {
				rr := r
				p.genResult = &rr
			}
		case now := <-tick.C:
			dt := now.Sub(last).Seconds()
			last = now
			p.step(dt, now)
			p.tickN++
			if p.tickN%hotEvery == 0 {
				p.broadcastHot()
			}
			if p.tickN%movesEvery == 0 {
				p.broadcastMoves()
			}
			if p.tickN%coldEvery == 0 {
				p.broadcastCold()
				p.broadcastSelf()
			}
		}
	}
}

// --- public commands -------------------------------------------------------

// Join registers a connection for sid with its account + appearance. accountID
// is 0 for anon; name/shape "" and bc/sk < 0 are derived from the id.
func (p *Pond) Join(sid string, accountID int64, name, shape string, bc, sk int) {
	p.cmds <- func() { p.join(sid, accountID, name, shape, bc, sk) }
}

func (p *Pond) Leave(sid string) { p.cmds <- func() { p.leave(sid) } }

// Move sets the visitor's floor position (clamped into the room, outside the tank).
func (p *Pond) Move(sid string, x, z float64) { p.cmds <- func() { p.move(sid, x, z) } }

// Ask enqueues sid; spendFood buys priority; msg is an optional (untrusted)
// message the Oracle may react to. Cheers the Muskie up.
func (p *Pond) Ask(sid string, spendFood bool, msg string) {
	p.cmds <- func() { p.ask(sid, spendFood, msg) }
}

// DropFood spends one food to drop a pellet near (x,z) on the surface.
func (p *Pond) DropFood(sid string, x, z float64) { p.cmds <- func() { p.dropFood(sid, x, z) } }

// SetAvatar updates a logged-in visitor's appearance live (from the customize
// handler); empty/negative fields are left unchanged.
func (p *Pond) SetAvatar(accountID int64, shape string, bc, sk int) {
	p.cmds <- func() { p.setAvatar(accountID, shape, bc, sk) }
}

func (p *Pond) join(sid string, accountID int64, name, shape string, bc, sk int) {
	u := p.users[sid]
	if u == nil {
		ang := goldenAngle * float64(p.joinCount)
		u = &user{short: p.nextShort, name: randomName(sid), food: startFood}
		u.pos = p.ringPos(ang)
		p.nextShort++
		p.joinCount++
		p.users[sid] = u
	}
	u.conns++
	u.accountID = accountID
	if name != "" {
		u.name = name
	}
	if shape != "" {
		u.shape = shape
	}
	u.bc = resolveIdx(bc, sid, "b", bodyPaletteLen)
	u.sk = resolveIdx(sk, sid, "s", skinPaletteLen)
	p.publishSelf(sid)
}

func (p *Pond) leave(sid string) {
	u := p.users[sid]
	if u == nil {
		return
	}
	u.conns--
	if u.conns > 0 {
		return
	}
	delete(p.users, sid)
	p.dequeue(sid)
	if p.serving == sid {
		p.serving = ""
		p.state = "roam"
		p.awaiting = false
	}
}

func (p *Pond) move(sid string, x, z float64) {
	u := p.users[sid]
	if u == nil {
		return
	}
	u.pos = p.clampFloor(x, z)
}

func (p *Pond) ask(sid string, spendFood bool, msg string) {
	u := p.users[sid]
	if u == nil || u.queued || p.serving == sid {
		return
	}
	u.askMsg = sanitizeMsg(msg)
	u.priority = 0
	if spendFood && u.food > 0 {
		u.food--
		u.priority = 1
	}
	u.seq = p.nextSeq
	p.nextSeq++
	u.queued = true
	p.queue = append(p.queue, sid)
	p.sortQueue()
	p.happy = clampStat(p.happy + askHappy)
}

func (p *Pond) dropFood(sid string, x, z float64) {
	u := p.users[sid]
	if u == nil || u.food <= 0 {
		return
	}
	x = clamp(x, -(p.curHalfW - margin), p.curHalfW-margin)
	z = clamp(z, -(p.curHalfD - margin), p.curHalfD-margin)
	u.food--
	p.food = append(p.food, pellet{id: p.nextPellet, x: x, y: foodY, z: z})
	p.nextPellet++
	if len(p.food) > maxPellets {
		p.food = p.food[len(p.food)-maxPellets:]
	}
	if u.accountID > 0 && p.rec != nil {
		p.rec.Fed(u.accountID)
	}
}

func (p *Pond) setAvatar(accountID int64, shape string, bc, sk int) {
	for sid, u := range p.users {
		if u.accountID != accountID {
			continue
		}
		if shape != "" {
			u.shape = shape
		}
		if bc >= 0 {
			u.bc = bc
		}
		if sk >= 0 {
			u.sk = sk
		}
		p.publishSelf(sid)
	}
}

// --- per-tick simulation ---------------------------------------------------

func (p *Pond) step(dt float64, now time.Time) {
	p.t += dt
	p.growTank(dt)
	p.accrueFood(dt)
	p.updateStats(dt)

	switch p.state {
	case "approach":
		if p.awaiting {
			// Arrived; hover facing the asker while the oracle line generates,
			// then speak it (or fall back) once ready or the grace expires.
			if u := p.users[p.serving]; u != nil {
				p.faceToward(u.pos, dt)
			}
			if p.speakReady() || now.After(p.awaitUntil) {
				p.beginSpeak(now)
			}
		} else {
			p.moveToward(p.target, travelSpeed, dt)
			if p.arrived(p.target) {
				if p.speakReady() {
					p.beginSpeak(now)
				} else {
					p.awaiting = true
					p.awaitUntil = now.Add(oracleGraceMs)
				}
			}
		}
	case "speak":
		if u := p.users[p.serving]; u != nil {
			p.faceToward(u.pos, dt)
		}
		if now.After(p.speakUntil) {
			p.serving = ""
			p.state = "roam"
		}
	case "sleep":
		p.moveToward(vec3{p.curHalfW * 0.4, restY, p.curHalfD * 0.4}, sleepSpeed, dt)
		if p.energy >= sleepExit {
			p.state = "roam"
		}
	case "graze":
		if p.hasQueue() {
			p.beginApproach()
			return
		}
		pel, ok := p.pelletByID(p.grazeID)
		if !ok {
			p.state = "roam"
			return
		}
		tgt := vec3{pel.x, pel.y, pel.z}
		p.moveToward(tgt, grazeSpeed, dt)
		if p.arrived(tgt) {
			p.eat(p.grazeID)
			p.state = "roam"
		}
	default: // roam
		switch {
		case p.energy <= sleepEnter:
			p.state = "sleep"
		case p.hasQueue():
			p.beginApproach()
		case p.hasFood():
			id, _ := p.nearestFood()
			p.grazeID = id
			p.state = "graze"
		default:
			p.target = p.roamPoint()
			p.moveToward(p.target, roamSpeed, dt)
		}
	}
}

func (p *Pond) growTank(dt float64) {
	tw, td := p.tankTarget()
	e := math.Min(1, dt*tankEase)
	p.curHalfW += (tw - p.curHalfW) * e
	p.curHalfD += (td - p.curHalfD) * e
}

func (p *Pond) tankTarget() (float64, float64) {
	s := clamp(math.Sqrt(float64(p.countConnected())/refUsers), 1, maxScale)
	return baseHalfW * s, baseHalfD * s
}

func (p *Pond) countConnected() int {
	n := 0
	for _, u := range p.users {
		if u.conns > 0 {
			n++
		}
	}
	return n
}

func (p *Pond) updateStats(dt float64) {
	p.hunger = clampStat(p.hunger + hungerRise*dt)
	p.happy = clampStat(p.happy - happyDecay*dt)
	if p.hunger > 85 {
		p.happy = clampStat(p.happy - starveHappy*dt)
	}
	if p.state == "sleep" {
		p.energy = clampStat(p.energy + sleepRecover*dt)
	} else {
		p.energy = clampStat(p.energy - energyDrain*dt)
	}
}

func (p *Pond) accrueFood(dt float64) {
	for _, u := range p.users {
		if u.conns <= 0 || u.food >= foodCap {
			continue
		}
		u.accrual += dt
		for u.accrual >= accrualSecs && u.food < foodCap {
			u.accrual -= accrualSecs
			u.food++
		}
	}
}

func (p *Pond) beginApproach() {
	sid, ok := p.front()
	if !ok {
		return
	}
	p.queue = p.queue[1:]
	u := p.users[sid]
	if u == nil {
		return
	}
	u.queued = false
	p.serving = sid
	p.state = "approach"
	p.awaiting = false
	p.target = p.nearWall(angleOf(u.pos)) // come to the wall nearest where they stand
	p.startGenerate(u)                     // kick off the oracle line during the swim over
}

// startGenerate launches the oracle for the current asker (off the sim
// goroutine). The result lands on oracleCh, tagged with this generation id so a
// later ask supersedes it. A nil oracle leaves genResult nil → flat-file.
func (p *Pond) startGenerate(u *user) {
	p.genSeq++
	p.genResult = nil
	if p.oracle == nil {
		return
	}
	gen := p.genSeq
	req := SpeakRequest{
		Crowd:     p.countConnected(),
		Hunger:    int(p.hunger + 0.5),
		Energy:    int(p.energy + 0.5),
		Happy:     int(p.happy + 0.5),
		State:     p.state,
		AccountID: u.accountID,
		Name:      u.name,
		Message:   u.askMsg,
	}
	oracle := p.oracle
	ch := p.oracleCh
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), oracleTimeout)
		defer cancel()
		utt, ok := oracle.Speak(ctx, req)
		ch <- oracleResult{gen: gen, utt: utt, ok: ok}
	}()
}

// speakReady reports whether the Muskie can speak now: no oracle in play, or the
// current generation's result has already landed.
func (p *Pond) speakReady() bool { return p.oracle == nil || p.genResult != nil }

// pickQuote returns the line to speak (and its voice): the oracle's, if it
// produced a usable one, otherwise a flat-file quote in the default voice.
func (p *Pond) pickQuote() (text, attr, voice string) {
	if p.genResult != nil && p.genResult.ok && strings.TrimSpace(p.genResult.utt.Text) != "" {
		return p.genResult.utt.Text, "The Muskie", p.genResult.utt.Voice
	}
	q, _ := p.wisdom.Random()
	return q.Text, q.Attribution, ""
}

func (p *Pond) beginSpeak(now time.Time) {
	p.awaiting = false
	p.quoteText, p.quoteAttr, p.quoteVoice = p.pickQuote()
	p.quotePhon = p.voice.Phonemes(p.quoteText)
	p.utter++
	p.state = "speak"
	p.speakUntil = now.Add(estSpeakDuration(p.quotePhon))
	if u := p.users[p.serving]; u != nil {
		u.askMsg = ""
		if u.accountID > 0 && p.rec != nil {
			p.rec.Wisdom(u.accountID)
		}
	}
}

func estSpeakDuration(phonemes string) time.Duration {
	ms := speakBaseMs + len(strings.Fields(phonemes))*speakPerTok
	if ms > speakMaxMs {
		ms = speakMaxMs
	}
	return time.Duration(ms) * time.Millisecond
}

func (p *Pond) eat(id int) {
	for i, f := range p.food {
		if f.id == id {
			p.food = append(p.food[:i], p.food[i+1:]...)
			p.hunger = clampStat(p.hunger - eatHunger)
			p.happy = clampStat(p.happy + eatHappy)
			return
		}
	}
}

func (p *Pond) moveToward(target vec3, speed, dt float64) {
	dx, dy, dz := target.X-p.pos.X, target.Y-p.pos.Y, target.Z-p.pos.Z
	dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if step := speed * dt; dist <= step || dist < 1e-9 {
		p.pos = target
	} else {
		p.pos.X += dx / dist * step
		p.pos.Y += dy / dist * step
		p.pos.Z += dz / dist * step
	}
	if math.Hypot(dx, dz) > 1e-4 {
		p.turnTo(headingYaw(dx, dz), dt)
	}
}

func (p *Pond) faceToward(at vec3, dt float64) {
	dx, dz := at.X-p.pos.X, at.Z-p.pos.Z
	if math.Hypot(dx, dz) > 1e-4 {
		p.turnTo(headingYaw(dx, dz), dt)
	}
}

func (p *Pond) turnTo(want, dt float64) { p.yaw = lerpAngle(p.yaw, want, math.Min(1, yawRate*dt)) }

func (p *Pond) arrived(target vec3) bool {
	return math.Hypot(target.X-p.pos.X, target.Z-p.pos.Z) <= arriveEps &&
		math.Abs(target.Y-p.pos.Y) <= arriveEps
}

func (p *Pond) roamPoint() vec3 {
	return vec3{
		X: math.Sin(p.t*0.17) * (p.curHalfW - margin - 0.3),
		Y: math.Sin(p.t*0.23) * 0.7,
		Z: math.Cos(p.t*0.13) * (p.curHalfD - margin - 0.3),
	}
}

func (p *Pond) hasQueue() bool { return len(p.queue) > 0 }
func (p *Pond) hasFood() bool  { return len(p.food) > 0 }

func (p *Pond) front() (string, bool) {
	if len(p.queue) == 0 {
		return "", false
	}
	return p.queue[0], true
}

func (p *Pond) dequeue(sid string) {
	for i, q := range p.queue {
		if q == sid {
			p.queue = append(p.queue[:i], p.queue[i+1:]...)
			return
		}
	}
}

func (p *Pond) queuePos(sid string) int {
	for i, q := range p.queue {
		if q == sid {
			return i + 1
		}
	}
	return 0
}

func (p *Pond) sortQueue() {
	sort.SliceStable(p.queue, func(i, j int) bool {
		a, b := p.users[p.queue[i]], p.users[p.queue[j]]
		if a == nil || b == nil {
			return false
		}
		if a.priority != b.priority {
			return a.priority > b.priority
		}
		return a.seq < b.seq
	})
}

func (p *Pond) nearestFood() (int, bool) {
	best, bestD := -1, math.MaxFloat64
	for _, f := range p.food {
		d := math.Hypot(f.x-p.pos.X, f.z-p.pos.Z)
		if d < bestD {
			best, bestD = f.id, d
		}
	}
	return best, best >= 0
}

func (p *Pond) pelletByID(id int) (pellet, bool) {
	for _, f := range p.food {
		if f.id == id {
			return f, true
		}
	}
	return pellet{}, false
}

func (p *Pond) ringRadius() float64 { return math.Max(p.curHalfW, p.curHalfD) + ringGap }
func (p *Pond) roomEdge() float64   { return p.ringRadius() + roomGap }

func (p *Pond) ringPos(angle float64) vec3 {
	r := p.ringRadius()
	return vec3{X: r * math.Cos(angle), Y: 0, Z: r * math.Sin(angle)}
}

// clampFloor keeps a floor position inside the room circle and outside the tank.
func (p *Pond) clampFloor(x, z float64) vec3 {
	if r := math.Hypot(x, z); r > p.roomEdge() {
		s := p.roomEdge() / r
		x, z = x*s, z*s
	}
	ex, ez := p.curHalfW+0.6, p.curHalfD+0.6
	if math.Abs(x) < ex && math.Abs(z) < ez {
		if ex-math.Abs(x) < ez-math.Abs(z) {
			x = math.Copysign(ex, x)
		} else {
			z = math.Copysign(ez, z)
		}
	}
	return vec3{X: x, Y: 0, Z: z}
}

// nearWall is the point just inside the glass nearest a direction angle.
func (p *Pond) nearWall(angle float64) vec3 {
	dx, dz := math.Cos(angle), math.Sin(angle)
	t := math.MaxFloat64
	if ax := math.Abs(dx); ax > 1e-6 {
		t = math.Min(t, (p.curHalfW-margin)/ax)
	}
	if az := math.Abs(dz); az > 1e-6 {
		t = math.Min(t, (p.curHalfD-margin)/az)
	}
	return vec3{X: dx * t, Y: 0, Z: dz * t}
}

func angleOf(p vec3) float64            { return math.Atan2(p.Z, p.X) }
func headingYaw(dx, dz float64) float64 { return math.Atan2(-dz, dx) }

func lerpAngle(a, b, t float64) float64 {
	d := math.Mod(b-a, 2*math.Pi)
	if d > math.Pi {
		d -= 2 * math.Pi
	}
	if d < -math.Pi {
		d += 2 * math.Pi
	}
	return a + d*t
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampStat(v float64) float64 { return clamp(v, 0, 100) }

// sanitizeMsg caps and flattens an untrusted visitor message to a single short
// line before it is ever handed to the Oracle (control chars dropped, newlines
// neutralized so it can't forge prompt structure).
func sanitizeMsg(s string) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > askMsgMax {
		r = r[:askMsgMax]
	}
	out := make([]rune, 0, len(r))
	for _, c := range r {
		switch {
		case c == '\n' || c == '\r' || c == '\t':
			out = append(out, ' ')
		case c < 0x20:
			// drop other control characters
		default:
			out = append(out, c)
		}
	}
	return strings.TrimSpace(string(out))
}

// resolveIdx returns provided if in [0,n), else a stable index derived from sid.
func resolveIdx(provided int, sid, salt string, n int) int {
	if provided >= 0 && provided < n {
		return provided
	}
	return int(hash32(sid+salt) % uint32(n))
}

// --- frames ----------------------------------------------------------------

type mState struct {
	X, Y, Z, Yaw float64 `json:"-"`
	St           string  `json:"st"`
	Srv          string  `json:"srv"`
	Ut           int     `json:"ut"`
}

func (m mState) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"x": round(m.X), "y": round(m.Y), "z": round(m.Z), "yaw": round(m.Yaw),
		"st": m.St, "srv": m.Srv, "ut": m.Ut,
	})
}

type statBlock struct {
	Hunger int `json:"hunger"`
	Energy int `json:"energy"`
	Happy  int `json:"happy"`
}

type hotQuote struct {
	T   string `json:"t"`
	A   string `json:"a"`
	P   string `json:"p"`
	V   string `json:"v,omitempty"` // voice name ("" = client default)
	Srv string `json:"srv"`
}

type hotFrame struct {
	K     string    `json:"k"`
	M     mState    `json:"m"`
	Stats statBlock `json:"stats"`
	Quote *hotQuote `json:"quote,omitempty"`
}

type coldUser struct {
	ID    string  `json:"id"`
	I     uint32  `json:"i"`
	N     string  `json:"n"`
	X     float64 `json:"x"`
	Z     float64 `json:"z"`
	Shape string  `json:"shape"`
	BC    int     `json:"bc"`
	SK    int     `json:"sk"`
}

type snapPellet struct {
	ID int     `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
	Z  float64 `json:"z"`
}

type tankDims struct {
	W float64 `json:"w"`
	D float64 `json:"d"`
}

type coldFrame struct {
	K      string       `json:"k"`
	Tank   tankDims     `json:"tank"`
	Count  int          `json:"count"`
	Others []coldUser   `json:"others"`
	Food   []snapPellet `json:"food"`
	Q      []string     `json:"q"`
	QLen   int          `json:"qlen"`
}

type selfFrame struct {
	K     string  `json:"k"`
	N     string  `json:"n"`
	I     uint32  `json:"i"`
	X     float64 `json:"x"`
	Z     float64 `json:"z"`
	Shape string  `json:"shape"`
	BC    int     `json:"bc"`
	SK    int     `json:"sk"`
	Food  int     `json:"food"`
	QPos  int     `json:"qpos"`
}

func (p *Pond) broadcastHot() {
	f := hotFrame{
		K:     "hot",
		M:     mState{X: p.pos.X, Y: p.pos.Y, Z: p.pos.Z, Yaw: p.yaw, St: p.state, Srv: p.serving, Ut: p.utter},
		Stats: statBlock{Hunger: int(p.hunger + 0.5), Energy: int(p.energy + 0.5), Happy: int(p.happy + 0.5)},
	}
	if p.state == "speak" {
		srvName := ""
		if u := p.users[p.serving]; u != nil {
			srvName = u.name
		}
		f.Quote = &hotQuote{T: p.quoteText, A: p.quoteAttr, P: p.quotePhon, V: p.quoteVoice, Srv: srvName}
	}
	if b, err := json.Marshal(f); err == nil {
		p.hub.Publish(Topic, hub.Event{Bytes: b})
	}
}

func (p *Pond) broadcastCold() {
	f := coldFrame{
		K:     "cold",
		Tank:  tankDims{W: round(p.curHalfW), D: round(p.curHalfD)},
		Count: p.countConnected(),
		QLen:  len(p.queue),
	}
	for sid, u := range p.users {
		if u.conns <= 0 {
			continue
		}
		if len(f.Others) >= sampleCap {
			break
		}
		f.Others = append(f.Others, coldUser{
			ID: sid, I: u.short, N: u.name, X: round(u.pos.X), Z: round(u.pos.Z),
			Shape: u.shape, BC: u.bc, SK: u.sk,
		})
	}
	for _, fd := range p.food {
		f.Food = append(f.Food, snapPellet{ID: fd.id, X: round(fd.x), Y: round(fd.y), Z: round(fd.z)})
	}
	n := len(p.queue)
	if n > queueCap {
		n = queueCap
	}
	f.Q = append([]string(nil), p.queue[:n]...)
	if b, err := json.Marshal(f); err == nil {
		p.hub.Publish(ColdTopic, hub.Event{Bytes: b})
	}
}

// broadcastMoves publishes the sampled visitors' positions as a compact array
// of [shortID, x, z] triples.
func (p *Pond) broadcastMoves() {
	m := make([][3]float64, 0, sampleCap)
	for _, u := range p.users {
		if u.conns <= 0 {
			continue
		}
		if len(m) >= sampleCap {
			break
		}
		m = append(m, [3]float64{float64(u.short), round(u.pos.X), round(u.pos.Z)})
	}
	b, err := json.Marshal(map[string]any{"k": "moves", "m": m})
	if err == nil {
		p.hub.Publish(MovesTopic, hub.Event{Bytes: b})
	}
}

func (p *Pond) broadcastSelf() {
	for sid, u := range p.users {
		if u.conns > 0 {
			p.publishSelf(sid)
		}
	}
}

func (p *Pond) publishSelf(sid string) {
	u := p.users[sid]
	if u == nil {
		return
	}
	f := selfFrame{
		K: "self", N: u.name, I: u.short, X: round(u.pos.X), Z: round(u.pos.Z),
		Shape: u.shape, BC: u.bc, SK: u.sk, Food: u.food, QPos: p.queuePos(sid),
	}
	if b, err := json.Marshal(f); err == nil {
		p.hub.Publish(SelfTopic(sid), hub.Event{Bytes: b})
	}
}

func round(v float64) float64 { return math.Round(v*1000) / 1000 }
