package sim

import (
	"github.com/ThirdCoastInteractive/the-beach/ecs"
	"github.com/ThirdCoastInteractive/the-beach/hub"
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
