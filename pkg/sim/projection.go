package sim

import (
	"github.com/ThirdCoastInteractive/Beach/pkg/ecs"
	"github.com/ThirdCoastInteractive/Beach/pkg/hub"
)

// A Projection maps a changed component onto a hub topic and a rendered patch.
// It is the event-sourcing read-model: each projection pass collects every
// entity whose component T was stamped since the last pass (ecs.Changed[T]),
// and for each one decides which topic it belongs to and what bytes to publish.
//
// View returns the already-rendered patch bytes (typically a
// datastar-patch-elements SSE frame). The sim stays free of templ: render to
// []byte in the app layer and hand the function in. A nil/empty return skips
// publishing for that entity.
//
// "Render once per topic": within a single pass, the sim renders each distinct
// topic at most once. If two entities map to the same topic in the same pass,
// the last one wins (it is the freshest state for that surface). This is the
// interest-management dedupe — a per-user XP bar topic should emit one frame per
// pass no matter how many of that user's components changed.
type Projection[T any] struct {
	// Topic maps an (entity, component) to the hub topic it publishes on.
	// Returning "" drops the entity from this pass (not interested / not ready).
	Topic func(e ecs.Entity, v T) string
	// View renders the patch bytes for this (entity, component). It runs on the
	// loop goroutine; keep it allocation-light. Returning nil skips publishing.
	View func(e ecs.Entity, v T) []byte
}

// projection is the type-erased registration the sim loop iterates. collect is
// the per-pass closure that runs ecs.Changed[T], maps to topics, renders once
// per topic, and writes results into the shared dedupe map.
type projection struct {
	collect func(s *ecs.Store, since Tick, out map[string][]byte)
}

// Project registers a typed projection on the sim. Call before Run. Projections
// run in registration order, but because each pass dedupes by topic, order only
// matters when two different component projections target the same topic — then
// the later registration wins for that topic in a pass.
func Project[T any](s *Sim, p Projection[T]) {
	if p.Topic == nil || p.View == nil {
		panic("sim: Projection requires both Topic and View")
	}
	s.projs = append(s.projs, projection{
		collect: func(store *ecs.Store, since Tick, out map[string][]byte) {
			for e, v := range ecs.Changed[T](store, since) {
				topic := p.Topic(e, v)
				if topic == "" {
					continue
				}
				bytes := p.View(e, v)
				if bytes == nil {
					continue
				}
				out[topic] = bytes // render once per topic: last write wins
			}
		},
	})
}

// AnyProjection renders a whole surface when ANY member component changed in a
// pass — the dirty trigger is "any change within a component set," not one named
// component. It removes the implicit-coupling workaround where every mutating
// command must touch a singleton trigger component (boardwalk's touchBoard) just
// to flag the shared board dirty: list the components the surface derives from as
// Members, and a stamp on any one of them re-renders the surface.
//
// Unlike Projection[T], the callbacks receive only the changed entity, not a
// component value — a whole-surface render reads the state it needs itself
// (typically via an ecs.View or a closure over the store on the loop). Topic
// maps a changed entity to its surface topic (return "" to drop it); for a
// single shared surface return a constant. View renders that surface's patch
// bytes. Each distinct topic is rendered exactly once per pass: dirty entities
// are mapped to topics first and deduped, so a shared surface fed by many
// changed members renders (and publishes) a single frame, not one per entity.
// When several entities map to the same topic, the first encountered wins as
// that topic's representative.
type AnyProjection struct {
	// Topic maps a changed entity to the hub topic its surface publishes on.
	// Returning "" drops the entity from this pass. For one shared surface,
	// ignore the entity and return a constant topic.
	Topic func(e ecs.Entity) string
	// View renders the surface's patch bytes for a changed entity. It runs on the
	// loop goroutine; keep it allocation-light. Returning nil skips publishing.
	View func(e ecs.Entity) []byte
}

// Member names one component type in an AnyProjection's trigger set. Construct
// it with the generic MemberOf[T]; ProjectAny fires when any listed member's
// column was stamped since the last pass. It mirrors the Mirror[T] shape in the
// write-behind lane: a typed registration captured as a type-erased scan.
type Member struct {
	// changed scans the store for entities whose member component changed since
	// `since`, appending each to out (deduped by the caller).
	changed func(s *ecs.Store, since Tick, out map[ecs.Entity]struct{})
}

// MemberOf builds an AnyProjection trigger member for component T. List the
// members a surface derives from; a stamp on any of them re-renders the surface.
func MemberOf[T any]() Member {
	return Member{
		changed: func(s *ecs.Store, since Tick, out map[ecs.Entity]struct{}) {
			for e := range ecs.Changed[T](s, since) {
				out[e] = struct{}{}
			}
		},
	}
}

// ProjectAny registers a whole-surface projection that fires when any of the
// given member components changed since the last pass. Call before Run. It
// composes with Project: both feed the same per-topic dedupe map each pass, so a
// topic driven by both wins on the later registration for that pass. Panics if
// Topic or View is nil, or if no members are given (a projection that can never
// fire is a bug).
func ProjectAny(s *Sim, p AnyProjection, members ...Member) {
	if p.Topic == nil || p.View == nil {
		panic("sim: AnyProjection requires both Topic and View")
	}
	if len(members) == 0 {
		panic("sim: ProjectAny requires at least one Member")
	}
	s.projs = append(s.projs, projection{
		collect: func(store *ecs.Store, since Tick, out map[string][]byte) {
			// Union the dirty entities across every member, deduped so an entity
			// that changed in two member columns is considered once.
			dirty := make(map[ecs.Entity]struct{})
			for _, m := range members {
				m.changed(store, since, dirty)
			}
			// Map dirty entities to topics and keep one representative entity per
			// topic, so each surface renders exactly once this pass.
			reps := make(map[string]ecs.Entity)
			for e := range dirty {
				topic := p.Topic(e)
				if topic == "" {
					continue
				}
				if _, seen := reps[topic]; !seen {
					reps[topic] = e
				}
			}
			for topic, e := range reps {
				bytes := p.View(e)
				if bytes == nil {
					continue
				}
				out[topic] = bytes
			}
		},
	})
}

// project runs one projection pass: gather the dirty set since the last pass
// across every registered projection into a per-topic map (deduped), publish
// each topic's bytes once to the hub, then advance lastProjectTick and flush the
// write-behind lane.
func (s *Sim) project(w *World) {
	since := s.lastProjectTick
	s.lastProjectTick = w.tick

	// Render once per topic, then publish. With no hub there is nowhere to send,
	// so skip the work entirely — write-behind below scans the dirty set itself.
	if len(s.projs) > 0 && s.cfg.Hub != nil {
		patches := make(map[string][]byte)
		for _, p := range s.projs {
			p.collect(w.Store, since, patches)
		}
		for topic, bytes := range patches {
			s.cfg.Hub.Publish(topic, hub.Event{Bytes: bytes})
		}
	}

	if s.behind != nil {
		s.behind.flush(w.Store, since)
	}
}
