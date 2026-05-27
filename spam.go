package main

import (
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
)

// UserSpamState はユーザーのスパム状態を保持する構造体
type UserSpamState struct {
	LastMessageTime int64
	MessageCount    int
}

var userSpamStatus = make(map[string]*UserSpamState)

const (
	silenceWindowSeconds = 8  // この秒数以内に
	messageThreshold     = 10 // このメッセージ数で連投判定
)

// getDisplayName は表示名を優先度順に取得する（ニックネーム > グローバル表示名 > ユーザー名）
func getDisplayName(m *discordgo.MessageCreate) string {
	if m.Member != nil && m.Member.Nick != "" {
		return m.Member.Nick
	}
	if m.Author.GlobalName != "" {
		return m.Author.GlobalName
	}
	return m.Author.Username
}

// checkSpam はメッセージがスパムかどうかをチェックし、必要に応じてタイムアウトを実行する
func checkSpam(s *discordgo.Session, m *discordgo.MessageCreate) {
	now := time.Now().Unix()
	state, exists := userSpamStatus[m.Author.ID]

	if exists && now-state.LastMessageTime < int64(silenceWindowSeconds) {
		// 時間枠内のメッセージ、カウント増加
		state.MessageCount++
	} else {
		// 新しい時間枠、カウントリセット
		userSpamStatus[m.Author.ID] = &UserSpamState{
			LastMessageTime: now,
			MessageCount:    1,
		}
		state = userSpamStatus[m.Author.ID]
	}

	// 連投判定
	if state.MessageCount >= messageThreshold {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("%sさんによる連投を検知しました\nタイムアウトします^^\n", getDisplayName(m)))

		timeoutUntil := time.Now().Add(30 * time.Minute)
		error := s.GuildMemberTimeout(m.GuildID, m.Author.ID, &timeoutUntil)
		if error != nil {
			log.Println("timeout setup error:", error)
		}

		delete(userSpamStatus, m.Author.ID)
	}
}
