package gui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// The editor's "Limits are per model" tick is saved on a provider, opens as
// saved, and is kept by a Save that leaves it out.
func TestProviderSavePerModelLimits(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	mux := http.NewServeMux()
	providerRoutes(mux, nil)
	post := func(body string) {
		t.Helper()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/provider/save", strings.NewReader(body)))
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
	on := func() bool {
		t.Helper()
		p, err := provider.Find("agg")
		if err != nil {
			t.Fatal(err)
		}
		if info := providerInfo(*p, nil); info.PerModelLimits != p.PerModelLimits {
			t.Fatalf("the editor is shown %v, saved %v", info.PerModelLimits, p.PerModelLimits)
		}
		return p.PerModelLimits
	}
	const edit = `{"id":"agg","from":"agg","name":"Aggregator","chat":"http://127.0.0.1:8317/v1","icon":"generic","models":[],"headers":{}`

	post(`{"id":"agg","name":"Aggregator","key":"sk-a","chat":"http://127.0.0.1:8317/v1","icon":"generic","new":true,"perModelLimits":true}`)
	if !on() {
		t.Fatal("added with it, saved off")
	}
	post(edit + `}`)
	if !on() {
		t.Fatal("a Save that left it out turned it off")
	}
	post(edit + `,"perModelLimits":false}`)
	if on() {
		t.Fatal("turned off, still on")
	}
	post(`{"id":"agg2","name":"Aggregator 2","key":"sk-b","chat":"http://127.0.0.1:8318/v1","icon":"generic","new":true}`)
	if p, _ := provider.Find("agg2"); p == nil || p.PerModelLimits {
		t.Fatal("added without it, saved on")
	}
}
