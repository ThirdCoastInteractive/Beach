package datastar

import "testing"

func TestNav(t *testing.T) {
	tests := []struct {
		name     string
		attr     Attr
		wantName string
		wantVal  string
	}{
		{
			"navigate",
			Navigate("/topology"),
			"data-on:click__prevent",
			"@get('/topology'); $path = '/topology'; globalThis.history.pushState(null, '', '/topology')",
		},
		{
			"navigate with extra",
			Navigate("/x", "$navOpen = false"),
			"data-on:click__prevent",
			"@get('/x'); $path = '/x'; globalThis.history.pushState(null, '', '/x'); $navOpen = false",
		},
		{
			"popstate",
			PopstateNav(),
			"data-on:popstate__window",
			"@get(globalThis.location.pathname + globalThis.location.search); $path = globalThis.location.pathname",
		},
		{
			"active when",
			ActiveWhen("active", "/topology"),
			"data-class:active",
			"$path === '/topology'",
		},
		{
			"path signal",
			PathSignal(),
			"data-signals:path",
			"globalThis.location.pathname",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.attr.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", tt.attr.Name, tt.wantName)
			}
			if tt.attr.Val != tt.wantVal {
				t.Errorf("Val = %q, want %q", tt.attr.Val, tt.wantVal)
			}
		})
	}
}
