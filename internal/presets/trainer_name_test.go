package presets

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type trainerTransport func(*http.Request) (*http.Response, error)

func (f trainerTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestResolveTrainerName(t *testing.T) {
	original := steamAPIClient
	t.Cleanup(func() { steamAPIClient = original })
	steamAPIClient = &http.Client{Transport: trainerTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"items":[]}`)), Header: http.Header{}}, nil
	})}
	for _, tc := range []struct{ id, name, want string }{
		{"2358720", "自定义名字", "Black Myth: Wukong"},
		{"1245620", "My backup", "Elden Ring"},
		{"", " 黑神话：悟空 ", "Black Myth: Wukong"},
		{"", "赛博朋克2077", "Cyberpunk 2077"},
		{"", "Unknown Chinese 中文", ""},
		{"", "未知中文游戏", ""},
		{"", "Black Myth: Wukong", "Black Myth: Wukong"},
		{"bad&appids=1", "Game", ""},
	} {
		if got := ResolveTrainerName(context.Background(), tc.id, tc.name); got != tc.want {
			t.Errorf("%+v: got %q", tc, got)
		}
	}
}

func TestChineseSteamSearchRequiresUniqueExactTitle(t *testing.T) {
	original := steamAPIClient
	t.Cleanup(func() { steamAPIClient = original })
	for _, tc := range []struct{ body, want string }{
		{`{"items":[{"type":"app","name":"博德之门3","id":1086940}]}`, "1086940"},
		{`{"items":[{"type":"app","name":"博德之门3 豪华版","id":1}]}`, ""},
		{`{"items":[{"type":"app","name":"博德之门3","id":1},{"type":"app","name":"博德之门3","id":2}]}`, ""},
		{`{"items":[{"type":"bundle","name":"博德之门3","id":1}]}`, ""},
	} {
		steamAPIClient = &http.Client{Transport: trainerTransport(func(r *http.Request) (*http.Response, error) {
			if r.URL.Query().Get("term") != "博德之门3" || r.URL.Query().Get("l") != "schinese" {
				t.Fatalf("wrong search: %s", r.URL)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body)), Header: http.Header{}}, nil
		})}
		if got := searchSteamChineseAppID(context.Background(), "博德之门3"); got != tc.want {
			t.Fatalf("got %q want %q", got, tc.want)
		}
	}
}

func TestTrainerSteamLookupEnglishAndFailure(t *testing.T) {
	original := steamAPIClient
	t.Cleanup(func() { steamAPIClient = original })
	for _, tc := range []struct{ body, want string }{
		{`{"999999999":{"success":true,"data":{"name":"Official English Title"}}}`, "Official English Title"},
		{`{"999999999":{"success":true,"data":{"name":"中文游戏"}}}`, ""},
		{`{"999999999":{"success":false}}`, ""},
		{`invalid json`, ""},
	} {
		steamAPIClient = &http.Client{Transport: trainerTransport(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host != "store.steampowered.com" || r.URL.Query().Get("l") != "english" || r.URL.Query().Get("appids") != "999999999" {
				t.Fatalf("wrong request: %s", r.URL)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body)), Header: http.Header{}}, nil
		})}
		if got := ResolveTrainerName(context.Background(), "999999999", "中文"); got != tc.want {
			t.Fatalf("got %q, want %q", got, tc.want)
		}
	}
}

func TestTrainerChineseSearchThenEnglishLookup(t *testing.T) {
	original := steamAPIClient
	t.Cleanup(func() { steamAPIClient = original })
	calls := 0
	steamAPIClient = &http.Client{Transport: trainerTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		body := `{"items":[{"type":"app","name":"测试中文游戏","id":999999999}]}`
		if calls == 2 {
			if r.URL.Query().Get("l") != "english" || r.URL.Query().Get("appids") != "999999999" {
				t.Fatalf("wrong lookup: %s", r.URL)
			}
			body = `{"999999999":{"success":true,"data":{"name":"Real English Game Title"}}}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}
	if got := ResolveTrainerName(context.Background(), "", "测试中文游戏"); got != "Real English Game Title" || calls != 2 {
		t.Fatalf("got %q in %d calls", got, calls)
	}
}
