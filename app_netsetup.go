package main

import (
	"errors"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/netsetup"
)

// failure is what the status tells of err, a failed write of the system
// proxy or the system's DNS servers: its first line, unless the Mac asks
// for an administrator password for system-wide settings. networksetup
// then only says it needs admin privileges, and retrying changes nothing,
// so the status says where the option is (issue 189).
func failure(err error) string {
	if errors.Is(err, netsetup.ErrAdmin) {
		return i18n.T("这台 Mac 要求输入管理员密码才能更改系统范围的设置。在「系统设置 › 隐私与安全性 › 高级…」里关掉第一个选项后重试。",
			"This Mac asks for an administrator password to change system-wide settings. Turn off “Require an administrator password to access system-wide settings” under System Settings › Privacy & Security › Advanced…, then try again.")
	}
	return firstLine(err.Error())
}
