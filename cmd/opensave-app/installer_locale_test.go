package main

import (
	"os"
	"strings"
	"testing"
)

func TestInstallerOffersChineseAndEnglish(t *testing.T) {
	raw, err := os.ReadFile("build/windows/installer/project.nsi")
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)
	for _, required := range []string{
		`MUI_LANGUAGE "English"`, `MUI_LANGUAGE "SimpChinese"`,
		`MUI_LANGDLL_DISPLAY`, `MUI_UNGETLANGUAGE`,
		`正在安装 WebView2 运行时`, `WAILS_WIN10_REQUIRED`,
		`WAILS_ARCHITECTURE_NOT_SUPPORTED`,
	} {
		if !strings.Contains(script, required) {
			t.Errorf("installer language feature missing: %s", required)
		}
	}
	if strings.Contains(script, `RMDir /r "$AppData\.opensave"`) {
		t.Fatal("uninstaller must preserve local saves and credentials")
	}
}
