package api

import (
	"github.com/opensave/opensave/internal/store"
	"net/http"
	"testing"
)

func TestTrainerNameTrackedChineseGame(t *testing.T) {
	ts := startTestServer(t)
	if err := ts.daemon.Store.CreateGame(store.Game{ID: "trainer-chinese", Name: "黑神话：悟空", SavePath: ts.saveDir, ActiveBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	resp, data := ts.do(t, http.MethodGet, "/api/games/trainer-chinese/trainer-name", nil)
	if resp.StatusCode != 200 || string(data["name"]) != `"Black Myth: Wukong"` {
		t.Fatalf("resolve: %d %s", resp.StatusCode, data["name"])
	}
	game, err := ts.daemon.Store.GetGame("trainer-chinese")
	if err != nil || game.Name != "黑神话：悟空" || game.AppID != "" {
		t.Fatalf("resolution changed identity: %+v %v", game, err)
	}
	resp, _ = ts.do(t, http.MethodGet, "/api/games/missing/trainer-name", nil)
	if resp.StatusCode != 404 {
		t.Fatalf("missing: %d", resp.StatusCode)
	}
}
