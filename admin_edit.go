package main

import (
	"fmt"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
)

type adminEditSession struct {
	TargetChannelID string
	TargetMessageID string
	OriginalContent string
	WorkingContent  string
}

var (
	adminEditSessions   = map[string]*adminEditSession{}
	adminEditSessionsMu sync.Mutex
)

const (
	adminEditCommitButtonCustomID = "admin_edit_commit"
	adminEditCancelButtonCustomID = "admin_edit_cancel"
	adminEditOpenModalButtonID    = "admin_edit_open_modal"
	adminEditModalCustomID        = "admin_edit_modal_submit"
	adminEditModalInputCustomID   = "admin_edit_modal_content"
)

func handleEditReplaceInput(s *discordgo.Session, m *discordgo.MessageCreate) {
	session := getEditSession(m.Author.ID)
	if session == nil {
		s.ChannelMessageSend(m.ChannelID, "編集セッションが見つかりません。/edit で開始してください。")
		return
	}

	lines := strings.Split(m.Content, "\n")
	applied := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		before, after, ok := strings.Cut(line, "=>")
		if !ok {
			s.ChannelMessageSend(m.ChannelID, "形式が不正です。`旧文 => 新文` 形式で送ってください。")
			return
		}

		before = strings.TrimSpace(before)
		after = strings.TrimSpace(after)
		if before == "" {
			s.ChannelMessageSend(m.ChannelID, "`旧文` が空です。`旧文 => 新文` 形式で送ってください。")
			return
		}

		if !strings.Contains(session.WorkingContent, before) {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("`%s` が現在の本文に見つかりませんでした。", before))
			return
		}

		session.WorkingContent = strings.ReplaceAll(session.WorkingContent, before, after)
		applied++
	}

	if applied == 0 {
		s.ChannelMessageSend(m.ChannelID, "置換行が見つかりませんでした。`旧文 => 新文` 形式で送ってください。")
		return
	}
	saveEditSession(m.Author.ID, session)

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("%d 件の置換を適用しました。下のボタンからの操作により確定、キャンセルを選択してください。", applied))
	sendEditActionPanel(s, m.ChannelID)
}

func handleAdminEditComponentInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) bool {
	customID := i.MessageComponentData().CustomID
	if customID != adminEditCommitButtonCustomID && customID != adminEditCancelButtonCustomID && customID != adminEditOpenModalButtonID {
		return false
	}

	userID := interactionUserID(i)

	if userID == "" || !isAdmin(userID) {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "この操作は管理者のみ実行できます。"},
		})
		return true
	}

	if customID == adminEditOpenModalButtonID {
		handleAdminEditOpenModalInteraction(s, i, userID)
		return true
	}

	var (
		message string
		success bool
	)
	if customID == adminEditCommitButtonCustomID {
		message, success = commitEditSessionByUser(s, userID)
	} else {
		message, success = cancelEditSessionByUser(userID)
	}

	if success {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    message,
				Components: []discordgo.MessageComponent{},
			},
		})
		return true
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: message},
	})
	return true
}

func handleAdminEditOpenModalInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, userID string) {
	session := getEditSession(userID)
	if session == nil {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "編集セッションがありません。/edit で開始してください。"},
		})
		return
	}

	prefilled := session.WorkingContent
	if len(prefilled) > 2000 {
		prefilled = prefilled[:2000]
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			Title:    "メッセージを直接編集",
			CustomID: adminEditModalCustomID,
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    adminEditModalInputCustomID,
							Label:       "編集後の本文",
							Style:       discordgo.TextInputParagraph,
							Required:    true,
							MaxLength:   2000,
							Value:       prefilled,
							Placeholder: "ここで直接、追記や修正ができます",
						},
					},
				},
			},
		},
	})
}

func handleAdminEditModalSubmitInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) bool {
	if i.ModalSubmitData().CustomID != adminEditModalCustomID {
		return false
	}

	userID := interactionUserID(i)
	if userID == "" || !isAdmin(userID) {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "この操作は管理者のみ実行できます。"},
		})
		return true
	}

	session := getEditSession(userID)
	if session == nil {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "編集セッションがありません。/edit で開始してください。"},
		})
		return true
	}

	updatedText, err := getAdminEditModalContent(i)
	if err != nil {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "入力エラー: " + err.Error()},
		})
		return true
	}

	if strings.TrimSpace(updatedText) == "" {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "空メッセージにはできません。"},
		})
		return true
	}

	session.WorkingContent = updatedText
	saveEditSession(userID, session)

	preview := strings.Join([]string{
		"本文を更新しました。",
		"",
		"変更後プレビュー:",
		asCodeBlock(session.WorkingContent),
		"",
		"下のボタンから確定/キャンセルできます。",
	}, "\n")

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content:    truncateForDiscord(preview, 1900),
			Components: editActionComponents(),
		},
	})
	return true
}

func getAdminEditModalContent(i *discordgo.InteractionCreate) (string, error) {
	for _, comp := range i.ModalSubmitData().Components {
		row, ok := comp.(*discordgo.ActionsRow)
		if !ok {
			continue
		}
		for _, rowComp := range row.Components {
			input, ok := rowComp.(*discordgo.TextInput)
			if !ok {
				continue
			}
			if input.CustomID == adminEditModalInputCustomID {
				return input.Value, nil
			}
		}
	}

	return "", fmt.Errorf("編集本文の入力欄が見つかりませんでした")
}

func interactionUserID(i *discordgo.InteractionCreate) string {
	if i == nil {
		return ""
	}
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

func commitEditSessionByUser(s *discordgo.Session, userID string) (string, bool) {
	session := getEditSession(userID)
	if session == nil {
		return "編集セッションがありません。/edit で開始してください。", false
	}

	if strings.TrimSpace(session.WorkingContent) == "" {
		return "空メッセージにはできません。置換内容を見直してください。", false
	}

	_, err := s.ChannelMessageEdit(session.TargetChannelID, session.TargetMessageID, session.WorkingContent)
	if err != nil {
		return "編集に失敗しました。権限と対象メッセージを確認してください。", false
	}

	deleteEditSession(userID)
	return "メッセージを編集しました。", true
}

func cancelEditSessionByUser(userID string) (string, bool) {
	if !hasActiveEditSession(userID) {
		return "キャンセルする編集セッションがありません。", false
	}
	deleteEditSession(userID)
	return "編集セッションをキャンセルしました。", true
}

func sendEditActionPanel(s *discordgo.Session, channelID string) {
	_, _ = s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content:    "操作パネル: ボタンで確定/キャンセルできます。",
		Components: editActionComponents(),
	})
}

func editActionComponents() []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "本文を直接編集",
					Style:    discordgo.PrimaryButton,
					CustomID: adminEditOpenModalButtonID,
				},
				discordgo.Button{
					Label:    "確定",
					Style:    discordgo.SuccessButton,
					CustomID: adminEditCommitButtonCustomID,
				},
				discordgo.Button{
					Label:    "キャンセル",
					Style:    discordgo.DangerButton,
					CustomID: adminEditCancelButtonCustomID,
				},
			},
		},
	}
}

func hasActiveEditSession(userID string) bool {
	adminEditSessionsMu.Lock()
	defer adminEditSessionsMu.Unlock()
	_, ok := adminEditSessions[userID]
	return ok
}

func getEditSession(userID string) *adminEditSession {
	adminEditSessionsMu.Lock()
	defer adminEditSessionsMu.Unlock()
	session, ok := adminEditSessions[userID]
	if !ok {
		return nil
	}
	copySession := *session
	return &copySession
}

func saveEditSession(userID string, session *adminEditSession) {
	adminEditSessionsMu.Lock()
	defer adminEditSessionsMu.Unlock()
	adminEditSessions[userID] = session
}

func deleteEditSession(userID string) {
	adminEditSessionsMu.Lock()
	defer adminEditSessionsMu.Unlock()
	delete(adminEditSessions, userID)
}

func asCodeBlock(text string) string {
	clean := strings.ReplaceAll(text, "```", "'''")
	return "```\n" + truncateForDiscord(clean, 1200) + "\n```"
}

func truncateForDiscord(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	if limit <= 3 {
		return text[:limit]
	}
	return text[:limit-3] + "..."
}
