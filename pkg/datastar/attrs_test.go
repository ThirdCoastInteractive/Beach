package datastar

import (
	"testing"

	"github.com/a-h/templ"
)

func TestAttrString(t *testing.T) {
	tests := []struct {
		name string
		attr Attr
		want string
	}{
		{"on click", OnClick("@post('/x')"), `data-on:click="@post(&#39;/x&#39;)"`},
		{"on submit", OnSubmit("@post('/save')"), `data-on:submit="@post(&#39;/save&#39;)"`},
		{"on keydown", On("keydown", "$go()"), `data-on:keydown="$go()"`},
		{"on interval", OnInterval("5s", "@get('/x')"), `data-on-interval__duration.5s="@get(&#39;/x&#39;)"`},
		{"on interval leading", OnInterval("5s.leading", "@get('/x')"), `data-on-interval__duration.5s.leading="@get(&#39;/x&#39;)"`},
		{"on interval default", OnInterval("", "@get('/x')"), `data-on-interval="@get(&#39;/x&#39;)"`},
		{"bind", Bind("email"), `data-bind:email`},
		{"bind nested", Bind("form.qty"), `data-bind:form.qty`},
		{"show", Show("$open"), `data-show="$open"`},
		{"text", Text("$count"), `data-text="$count"`},
		{"signals map", Signals(map[string]any{"open": false}), `data-signals="{&#34;open&#34;:false}"`},
		{"signals expr", SignalsExpr("{count: 0}"), `data-signals="{count: 0}"`},
		{"signal named", Signal("open", true), `data-signals:open="true"`},
		{"class", Class(map[string]string{"active": "$open"}), `data-class="{&#34;active&#34;:&#34;$open&#34;}"`},
		{"attr bind", AttrBind("disabled", "$busy"), `data-attr:disabled="$busy"`},
		{"indicator", Indicator("loading"), `data-indicator:loading`},
		{"ref", Ref("dialog"), `data-ref:dialog`},
		{"escapes quotes", On("click", `say "hi"`), `data-on:click="say &#34;hi&#34;"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.attr.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAttrsString(t *testing.T) {
	as := Attrs{Bind("email"), OnClick("@post('/x')"), Show("$ok")}
	want := `data-bind:email data-on:click="@post(&#39;/x&#39;)" data-show="$ok"`
	if got := as.String(); got != want {
		t.Errorf("Attrs.String() = %q, want %q", got, want)
	}
}

func TestAttrsTempl(t *testing.T) {
	as := Attrs{Bind("email"), OnClick("@post('/x')")}
	got := as.Templ()
	want := templ.Attributes{
		"data-bind:email": true,
		"data-on:click":   "@post('/x')",
	}
	if len(got) != len(want) {
		t.Fatalf("Templ() len = %d, want %d", len(got), len(want))
	}
	for k, wv := range want {
		gv, ok := got[k]
		if !ok {
			t.Errorf("Templ() missing key %q", k)
			continue
		}
		if gv != wv {
			t.Errorf("Templ()[%q] = %v, want %v", k, gv, wv)
		}
	}
}

func TestSignalsMarshalError(t *testing.T) {
	// A channel cannot be JSON-marshalled; we must degrade to an empty object,
	// never leak unescaped or partial markup.
	got := Signals(make(chan int)).Val
	if got != "{}" {
		t.Errorf("Signals(chan) Val = %q, want %q", got, "{}")
	}
}
