// Package i18n holds the interface's language. Strings are written in
// place as pairs, T("中文", "English"), so each reads where it is used.
// Other languages are tables keyed by the Chinese of the pair (ja.go,
// hant.go); a string missing from them falls back to English for Japanese
// and to Simplified Chinese for Traditional.
package i18n

import (
	"runtime"
	"sync/atomic"
)

// Languages are the interface's languages, in the order the settings list them.
var Languages = []string{"zh", "zh-Hant", "en", "ja"}

// Names are the languages as the settings show them, each in itself.
var Names = map[string]string{"zh": "简体中文", "zh-Hant": "繁體中文", "en": "English", "ja": "日本語"}

const (
	zh int32 = iota
	en
	ja
	hant
)

var lang atomic.Int32

// Known says whether l is one of Languages.
func Known(l string) bool {
	_, ok := Names[l]
	return ok
}

// Set changes the interface's language. Unknown languages are Chinese.
func Set(l string) {
	switch l {
	case "en":
		lang.Store(en)
	case "ja":
		lang.Store(ja)
	case "zh-Hant":
		lang.Store(hant)
	default:
		lang.Store(zh)
	}
}

// Lang is the interface's language, one of Languages.
func Lang() string {
	switch lang.Load() {
	case en:
		return "en"
	case ja:
		return "ja"
	case hant:
		return "zh-Hant"
	}
	return "zh"
}

// En says whether the interface is in English.
func En() bool { return lang.Load() == en }

// windowsTable rephrases strings the macOS build phrases around a Mac
// for the Windows build, which asks for the administrator its own way
// and names its own system. Each key is the macOS pair's zh string; each
// value is the string in each language, in the constants' order.
var windowsTable = map[string][4]string{
	"未使用 · 第一次开启时，macOS 会请求管理员授权": {
		"未使用 · 第一次开启时，Windows 会请求管理员授权",
		"Not in use · The first time it's turned on, Windows asks for an administrator's authorization",
		"未使用 · 初めてオンにするとき、Windows が管理者の許可を求めます",
		"未使用 · 第一次開啟時，Windows 會要求管理者授權",
	},
	"macOS 的 TUN 网卡名要以 utun 开头, 如 utun1024": {
		"TUN 网卡名，留空用默认",
		"The TUN device's name; leave it empty for the default",
		"TUN インターフェイス名。空欄なら既定の名前を使います",
		"TUN 網卡名稱，留空用預設",
	},
	"浅色、深色，或跟随 Mac 的设置": {
		"浅色、深色，或跟随系统的设置",
		"Light, dark, or as the system is set",
		"ライト、ダーク、またはシステムの設定に従う",
		"淺色、深色，或跟隨系統的設定",
	},
	"登录 Mac 时自动打开 bowel": {
		"登录 Windows 时自动打开 bowel",
		"Open bowel when you log in to Windows",
		"Windows にログインしたときに bowel を自動的に開きます",
		"登入 Windows 時自動打開 bowel",
	},
	"用户名:密码；局域网设备要先登录，本机不用": {
		"用户名:密码；局域网设备要先登录，本机不用",
		"user:password; LAN devices sign in first, this computer never does",
		"ユーザー名:パスワード。LAN デバイスは先にログインが必要で、この PC は不要です",
		"使用者名稱:密碼；區域網路裝置要先登入，本機不用",
	},
}

// T is the pair's string in the interface's language.
func T(zhs, ens string) string {
	l := lang.Load()
	if runtime.GOOS == "windows" {
		if s, ok := windowsTable[zhs]; ok {
			if v := s[l]; v != "" {
				return v
			}
		}
	}
	switch l {
	case en:
		return ens
	case ja:
		if s, ok := jaTable[zhs]; ok {
			return s
		}
		return ens
	case hant:
		if s, ok := hantTable[zhs]; ok {
			return s
		}
	}
	return zhs
}

// Every is the pair's string in each of Languages, for recognising a
// value saved in whichever language the interface showed then.
func Every(zhs, ens string) []string {
	out := []string{zhs, ens}
	if s, ok := jaTable[zhs]; ok {
		out = append(out, s)
	}
	if s, ok := hantTable[zhs]; ok {
		out = append(out, s)
	}
	return out
}
