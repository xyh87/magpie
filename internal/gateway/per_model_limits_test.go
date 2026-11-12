package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// A provider whose limits the vendor counts per model: a 429 for one model
// rests that model alone, and its siblings still answer. Before, a 429 whose
// words named no model rested the key, and every model on it waited out the
// vendor's Retry-After (Tencent Cloud's TokenHub TPM limit is one such).
func TestPerModelLimitsRestsTheModelOnly(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	restingUntil.Lock()
	restingUntil.m, restingUntil.note = map[string]time.Time{}, map[string]Rest{}
	restingUntil.Unlock()
	routed.Lock()
	routed.failures = map[string]int{}
	routed.Unlock()

	const (
		limited = "deepseek-v4.1-flash-1017"
		sibling = "minimax-m3.1-flash"
	)
	var mu sync.Mutex
	var seen []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		seen = append(seen, req.Model)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if req.Model == limited {
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, `{"error":{"message":"You have exceeded your quota"}}`)
			return
		}
		io.WriteString(w, `{"id":"ok","choices":[]}`)
	}))
	defer up.Close()

	p := provider.Provider{ID: "relay", Name: "Relay", Chat: up.URL + "/v1",
		Models: []string{limited, sibling}, Key: "k-one", PerModelLimits: true}
	if err := provider.Save(p); err != nil {
		t.Fatal(err)
	}
	saved, err := provider.Find("relay")
	if err != nil {
		t.Fatal(err)
	}

	s := New()
	send := func(model string) (int, string) {
		t.Helper()
		rec := httptest.NewRecorder()
		body := `{"model":"relay/` + model + `","messages":[{"role":"user","content":"hi"}]}`
		s.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body)))
		return rec.Code, rec.Body.String()
	}

	if code, body := send(limited); code != http.StatusTooManyRequests {
		t.Fatalf("%s: %d %s", limited, code, body)
	}
	key := saved.RestKey(saved.KeyList()[0])
	if _, ok := RestOf(key); ok {
		t.Fatalf("the key rests whole for one model's limit")
	}
	r, ok := RestOf(key + "/" + limited)
	if !ok || r.Model != limited {
		t.Fatalf("%s doesn't rest on its own: %+v", limited, r)
	}
	if r.Why != failQuota {
		t.Fatalf("rest %+v, want a quota limit", r)
	}
	if left := time.Until(r.Until); left <= 0 || left > 16*time.Minute {
		t.Fatalf("rests %v, want about a quarter of an hour", left)
	}

	if code, body := send(sibling); code != http.StatusOK {
		t.Fatalf("%s, which the vendor didn't limit, answered %d: %s", sibling, code, body)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) < 2 || seen[0] != limited || seen[len(seen)-1] != sibling {
		t.Fatalf("upstream saw %q, want %q (possibly tried again) then %q", seen, limited, sibling)
	}
}

// A vendor's 429 whose words are a per-minute TPM limit but name no model
// id (Tencent Cloud TokenHub's, as the report had it) is the model's alone
// once the provider is set so, and reads as a rate limit rather than a
// plan's allowance.
func TestPerModelLimitsRestsAnUnnamedTPMRate(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	noRests()
	const model = "deepseek-v4.1-flash-1017"
	body := []byte(`{"error":{"message":"Token usage exceeds the current model TPM (tokens per minute) limit 1000000. Please reduce the request frequency or contact Tencent Cloud support to request a higher limit.","type":"invalid_request_error","code":"quota_exceeded"}}`)
	c := candidate{p: provider.Provider{ID: "tencent-cloud", Preset: "tencent-cloud", PerModelLimits: true}, model: model, rest: "tencent-cloud"}
	r := (&Server{}).restAfter(c, http.StatusTooManyRequests, http.Header{"Retry-After": {"1"}}, body)
	if r.Key != c.restID() || r.Model != model || r.Why != failRate {
		t.Fatalf("rest %+v, want %q (%s) as a rate limit", r, c.restID(), model)
	}
}

// The switch is the user's: off, a 429 that names no model rests the key
// whole as before; a credit refusal is the key's even when it is on.
func TestPerModelLimitsOffRestsTheKey(t *testing.T) {
	c := candidate{p: provider.Provider{ID: "relay"}, model: "m"}
	if modelLimited(c, failRate, []byte(`{"error":{"message":"Rate limit reached"}}`)) {
		t.Fatal("a provider not set to per-model limits rests one model")
	}
	c.p.PerModelLimits = true
	if !modelLimited(c, failRate, nil) {
		t.Fatal("a provider set to per-model limits rests its key instead")
	}
	if modelLimited(c, failCredit, nil) {
		t.Fatal("a credit refusal rests one model")
	}
}
