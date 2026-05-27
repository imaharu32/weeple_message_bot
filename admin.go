package main

import (
	"os"
	"strings"
)

var adminIDs = map[string]struct{}{}

// loadAdminIDs は環境変数から管理者IDを読み込む
func loadAdminIDs() {
	adminIDs = map[string]struct{}{}
	env := strings.TrimSpace(os.Getenv("BOT_ADMIN_IDS"))
	if env == "" {
		return
	}
	for _, id := range strings.Split(env, ",") {
		id = strings.TrimSpace(id)
		if id != "" {
			adminIDs[id] = struct{}{}
		}
	}
}

// isAdmin はユーザーが管理者かどうかを判定する
func isAdmin(userID string) bool {
	_, ok := adminIDs[userID]
	return ok
}
