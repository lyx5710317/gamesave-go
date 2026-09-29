//go:build windows || linux

package main

import "testing"

func TestTrayFollowsSelectedLanguage(t *testing.T) {
	a := NewApp()
	a.SetTrayLocale("zh-CN")
	if !a.trayChinese.Load() || labelsForTray(true).sync != "同步所有游戏" || labelsForTray(true).quit != "退出" {
		t.Fatal("Chinese tray labels not selected")
	}
	select {
	case <-a.trayLocaleChanged:
	default:
		t.Fatal("tray was not notified of language change")
	}
	a.SetTrayLocale("en")
	if a.trayChinese.Load() || labelsForTray(false).sync != "Sync all games" || labelsForTray(false).quit != "Quit" {
		t.Fatal("English tray labels not selected")
	}
}
