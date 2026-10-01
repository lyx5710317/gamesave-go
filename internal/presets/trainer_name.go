package presets

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

// Exact aliases only: fuzzy translations can select a different game or sequel.
var trainerChineseAppIDs = map[string]string{
	"黑神话：悟空": "2358720", "黑神话:悟空": "2358720", "黑神话悟空": "2358720",
	"艾尔登法环": "1245620", "赛博朋克2077": "1091500", "赛博朋克 2077": "1091500",
	"巫师3": "292030", "巫师3：狂猎": "292030", "巫师3:狂猎": "292030",
	"荒野大镖客2": "1174180", "荒野大镖客：救赎2": "1174180",
	"只狼": "814380", "只狼：影逝二度": "814380", "星露谷物语": "413150",
	"泰拉瑞亚": "105600", "空洞骑士": "367520", "哈迪斯": "1145360", "哈迪斯2": "1145350",
}

func englishTrainerName(name string) bool {
	for _, r := range name {
		if unicode.IsLetter(r) && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
			return false
		}
	}
	return strings.IndexFunc(name, func(r rune) bool { return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' }) >= 0
}

// ResolveTrainerName does not rename a tracked game or change its identity.
// A stored Steam ID outranks the display name, including custom English names.
func ResolveTrainerName(ctx context.Context, appID, name string) string {
	name = strings.TrimSpace(name)
	appID = strings.TrimSpace(appID)
	if appID == "" {
		appID = trainerChineseAppIDs[name]
		if appID == "" && strings.IndexFunc(name, func(r rune) bool { return unicode.Is(unicode.Han, r) }) >= 0 {
			appID = searchSteamChineseAppID(ctx, name)
		}
	}
	if appID != "" {
		for _, r := range appID {
			if r < '0' || r > '9' {
				return ""
			}
		}
		if title := popularSteamGames[appID]; englishTrainerName(title) {
			return title
		}
		var title string
		for _, game := range loadEmbeddedIndex() {
			if game.SteamID == appID && englishTrainerName(game.Name) {
				if title != "" && title != game.Name {
					title = ""
					break
				}
				title = game.Name
			}
		}
		if title != "" {
			return title
		}
		if title = fetchSteamAppNameLanguage(ctx, appID, "english"); englishTrainerName(title) {
			return strings.TrimSpace(title)
		}
		return ""
	}
	if englishTrainerName(name) {
		return name
	}
	return ""
}

// Require an exact, unique localized title. Search ranking is not identity.
func searchSteamChineseAppID(ctx context.Context, name string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://store.steampowered.com/api/storesearch/?l=schinese&cc=CN&term="+url.QueryEscape(name), nil)
	if err != nil {
		return ""
	}
	resp, err := steamAPIClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var result struct {
		Items []struct {
			Type, Name string
			ID         int64
		}
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1024*1024)).Decode(&result) != nil {
		return ""
	}
	id := ""
	for _, item := range result.Items {
		if item.Type == "app" && strings.TrimSpace(item.Name) == name && item.ID > 0 {
			candidate := strconv.FormatInt(item.ID, 10)
			if id != "" && id != candidate {
				return ""
			}
			id = candidate
		}
	}
	return id
}
