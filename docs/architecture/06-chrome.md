# Architecture 6 — `chrome`: components, tokens, no-pop-in

[← docs index](../README.md) · prev: [Cache & session](05-services.md) · next: [ECS engine](07-ecs.md)

Lifted interface/registry/context from `internal/view/tmpl/chrome`, unchanged in
spirit: `Chrome` interface with sub-interfaces (Layout, Buttons, Forms, Feedback,
Data, Overlays, Media, Messaging), immutable props structs carrying
`datastar.Attrs`, registration via `init()` + side-effect import, context injection
via middleware, `chrome.I("gear", chrome.Spin)` icon API with build-time FA subsetting.

The framework also owns the **semantic token contract**: surface slots
(`void/shade/panel/well`), ink (`ink/dim/ghost`), edges, and roles
(`accent/danger/success/info`) — plain `@theme {}` (never `@theme inline`; runtime
theme switching depends on it), theme classes applied on `<html>` in `<head>` before
paint. House rules ship as docs + vet checks: no hardcoded hex/OKLCH in class
attributes, no custom CSS rules, fonts vendored.

`chrome/driftwood` is the minimal reference implementation — enough to render the
skeleton app and prove the interface. RetroChrome remains manifold property.

**The no-pop-in rule is a framework law:** the first paint is the final paint. Server
renders final state including signal initial values; hidden things ship hidden; no
visible HTML is ever generated in client JS.
