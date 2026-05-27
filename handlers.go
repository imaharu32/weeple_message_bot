package main

import (
	"strings"

	"github.com/bwmarrin/discordgo"
)

// messageCreate はメッセージが作成された時の処理
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	// DM判定（GuildIDが空ならDM）
	isDM := m.GuildID == ""

	if isDM {
		handleDirectMessage(s, m)
		return
	}

	// スパムチェック
	checkSpam(s, m)
}

// handleDirectMessage はDMを処理する
func handleDirectMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	// テキストコマンドは使用しない
	// 編集中のセッションの => 形式の入力処理
	if hasActiveEditSession(m.Author.ID) && isAdmin(m.Author.ID) {
		handleEditReplaceInput(s, m)
	}
}

// interactionCreate はスラッシュコマンドとコンポーネント操作を処理する
func interactionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		handleApplicationCommand(s, i)
	case discordgo.InteractionMessageComponent:
		handleMessageComponent(s, i)
	case discordgo.InteractionModalSubmit:
		handleModalSubmit(s, i)
	}
}

// handleApplicationCommand はスラッシュコマンドを処理する
func handleApplicationCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	cmd := i.ApplicationCommandData()

	switch cmd.Name {
	case "say":
		handleSaySlashCommand(s, i)
	case "edit":
		handleEditSlashCommand(s, i)
	case "inspectmsg":
		handleInspectMsgSlashCommand(s, i)
	}
}

// handleSaySlashCommand は/sayスラッシュコマンドを処理する
func handleSaySlashCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// DMのみで実行可能
	if i.GuildID != "" {
		respondEphemeral(s, i, "このコマンドはDMのみで利用できます。")
		return
	}

	// 管理者チェック
	if !isAdmin(i.User.ID) {
		respondEphemeral(s, i, "このコマンドは管理者のみが使用できます。")
		return
	}

	cmd := i.ApplicationCommandData()

	// オプションから値を取得
	var channelID, message string
	for _, opt := range cmd.Options {
		switch opt.Name {
		case "channel_id":
			channelID = opt.StringValue()
		case "message":
			message = opt.StringValue()
		}
	}

	if channelID == "" || message == "" {
		respondEphemeral(s, i, "チャンネルIDとメッセージを指定してください。")
		return
	}

	// スラッシュコマンド入力では改行を直接入れにくいため、\n を改行として扱う。
	message = strings.ReplaceAll(message, `\n`, "\n")

	// メッセージを送信
	_, err := s.ChannelMessageSend(channelID, message)
	if err != nil {
		respondEphemeral(s, i, "送信に失敗しました。チャンネルIDと権限を確認してください。")
		return
	}

	respondEphemeral(s, i, "送信しました。")
}

// handleEditSlashCommand は/editスラッシュコマンドを処理する
func handleEditSlashCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// DMのみで実行可能
	if i.GuildID != "" {
		respondEphemeral(s, i, "このコマンドはDMのみで利用できます。")
		return
	}

	// 管理者チェック
	if !isAdmin(i.User.ID) {
		respondEphemeral(s, i, "このコマンドは管理者のみが使用できます。")
		return
	}

	cmd := i.ApplicationCommandData()

	// オプションから値を取得
	var channelID, messageID string
	for _, opt := range cmd.Options {
		switch opt.Name {
		case "channel_id":
			channelID = opt.StringValue()
		case "message_id":
			messageID = opt.StringValue()
		}
	}

	if channelID == "" || messageID == "" {
		respondEphemeral(s, i, "チャンネルIDとメッセージIDを指定してください。")
		return
	}

	// メッセージを取得
	targetMessage, err := s.ChannelMessage(channelID, messageID)
	if err != nil {
		respondEphemeral(s, i, "対象メッセージの取得に失敗しました。チャンネルIDとメッセージIDを確認してください。")
		return
	}

	// Botが送信したメッセージかチェック
	if targetMessage.Author == nil || targetMessage.Author.ID != s.State.User.ID {
		respondEphemeral(s, i, "Botが送信したメッセージのみ編集できます。")
		return
	}

	// 編集セッションを保存
	saveEditSession(i.User.ID, &adminEditSession{
		TargetChannelID: channelID,
		TargetMessageID: messageID,
		OriginalContent: targetMessage.Content,
		WorkingContent:  targetMessage.Content,
	})

	preview := "編集セッションを開始しました。\n\n" +
		"現在の本文:\n" +
		asCodeBlock(targetMessage.Content) + "\n\n" +
		"DMで `旧文 => 新文` 形式の置換を送るか、下のボタンから直接編集してください。"

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content:    truncateForDiscord(preview, 1900),
			Components: editActionComponents(),
			Flags:      discordgo.MessageFlagsEphemeral,
		},
	})
}

// handleInspectMsgSlashCommand は/inspectmsgスラッシュコマンドを処理する
func handleInspectMsgSlashCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// DMのみで実行可能
	if i.GuildID != "" {
		respondEphemeral(s, i, "このコマンドはDMのみで利用できます。")
		return
	}

	// 管理者チェック
	if !isAdmin(i.User.ID) {
		respondEphemeral(s, i, "このコマンドは管理者のみが使用できます。")
		return
	}

	cmd := i.ApplicationCommandData()

	// オプションから値を取得
	var channelID, messageID string
	for _, opt := range cmd.Options {
		switch opt.Name {
		case "channel_id":
			channelID = opt.StringValue()
		case "message_id":
			messageID = opt.StringValue()
		}
	}

	if channelID == "" || messageID == "" {
		respondEphemeral(s, i, "チャンネルIDとメッセージIDを指定してください。")
		return
	}

	// メッセージを取得
	targetMessage, err := s.ChannelMessage(channelID, messageID)
	if err != nil {
		respondEphemeral(s, i, "対象メッセージの取得に失敗しました。チャンネルIDとメッセージIDを確認してください。")
		return
	}

	// 検査レポートを生成
	reportChunks := buildMessageInspectionReport(targetMessage)

	// 最初のチャンクを ephemeral で返す
	if len(reportChunks) > 0 {
		respondEphemeral(s, i, reportChunks[0])

		// 追加のチャンクがあれば、DM チャンネルに送る
		if len(reportChunks) > 1 {
			dmChannel, err := s.UserChannelCreate(i.User.ID)
			if err == nil {
				for _, chunk := range reportChunks[1:] {
					s.ChannelMessageSend(dmChannel.ID, chunk)
				}
			}
		}
	} else {
		respondEphemeral(s, i, "検査対象メッセージが空です。")
	}
}

// handleMessageComponent はボタンやセレクトメニューを処理する
func handleMessageComponent(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// admin_edit.go のハンドラーで処理される
	handleAdminEditComponentInteraction(s, i)
}

// handleModalSubmit はモーダル送信を処理する
func handleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// admin_edit.go のハンドラーで処理される
	handleAdminEditModalSubmitInteraction(s, i)
}

func respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: msg,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}
