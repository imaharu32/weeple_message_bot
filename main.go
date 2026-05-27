package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

func main() {
	// .env ファイルを読み込む（エラーは無視：Docker では環境変数が既に設定されている）
	_ = godotenv.Load()

	// 管理者IDを読み込み
	loadAdminIDs()

	// Discord Botトークンを取得
	token := os.Getenv("DISCORD_BOT_TOKEN")
	if token == "" {
		fmt.Println("DISCORD_BOT_TOKENが設定されていません。")
		return
	}

	// Discord Botセッションを作成
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		fmt.Println("セッション作成エラー:", err)
		return
	}

	// イベントハンドラーを登録
	dg.AddHandler(messageCreate)
	dg.AddHandler(interactionCreate)

	// Intentsを設定（DM受信と本文取得を含む）
	dg.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentsDirectMessages |
		discordgo.IntentsMessageContent

	// Discordに接続
	err = dg.Open()
	if err != nil {
		fmt.Println("接続エラー:", err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := startCalendarSyncWorker(ctx, dg); err != nil {
		fmt.Println("カレンダー同期の起動に失敗:", err)
		return
	}

	fmt.Println("Botが起動しました。スラッシュコマンドを登録中...")

	// 登録するギルドIDを取得（空ならグローバル）
	guildID := os.Getenv("ALLOWED_GUILD_ID")
	if guildID != "" {
		fmt.Printf("スラッシュコマンドをギルド %s に限定登録します\n", guildID)
	} else {
		fmt.Println("スラッシュコマンドをグローバル登録します（全サーバーで利用可能）")
	}

	// スラッシュコマンドを登録
	commands := []*discordgo.ApplicationCommand{
		{
			Name:         "say",
			Description:  "指定チャンネルにメッセージを送信（DM専用、管理者のみ）",
			DMPermission: func(b bool) *bool { return &b }(true),
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "channel_id",
					Description: "送信先チャンネルID",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "message",
					Description: "送信するメッセージ",
					Required:    true,
				},
			},
		},
		{
			Name:         "edit",
			Description:  "メッセージを編集（DM専用、管理者のみ）",
			DMPermission: func(b bool) *bool { return &b }(true),
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "channel_id",
					Description: "メッセージがあるチャンネルID",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "message_id",
					Description: "編集するメッセージID",
					Required:    true,
				},
			},
		},
		{
			Name:         "inspectmsg",
			Description:  "メッセージの詳細情報を表示（DM専用、管理者のみ）",
			DMPermission: func(b bool) *bool { return &b }(true),
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "channel_id",
					Description: "メッセージがあるチャンネルID",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "message_id",
					Description: "検査するメッセージID",
					Required:    true,
				},
			},
		},
	}

	registeredCommands := make([]*discordgo.ApplicationCommand, len(commands))
	for i, cmd := range commands {
		// DM専用コマンドはグローバル登録、それ以外はguildID限定
		registerGuildID := guildID
		if cmd.Name == "say" || cmd.Name == "edit" || cmd.Name == "inspectmsg" {
			registerGuildID = "" // DM専用はグローバル登録
		}

		rcmd, err := dg.ApplicationCommandCreate(dg.State.User.ID, registerGuildID, cmd)
		if err != nil {
			fmt.Printf("コマンド '%s' の登録に失敗: %v\n", cmd.Name, err)
		} else {
			registeredCommands[i] = rcmd
			fmt.Printf("コマンド '%s' を登録しました\n", cmd.Name)
		}
	}

	fmt.Println("Botが起動しました。CTRL+Cで終了します。version 2025-2-16")
	defer dg.Close()

	// シグナル待機
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// 終了時にコマンドを削除
	fmt.Println("スラッシュコマンドを削除中...")
	for _, cmd := range registeredCommands {
		if cmd != nil {
			// DM専用コマンドはグローバルから削除、それ以外はguildIDから削除
			deleteGuildID := guildID
			if cmd.Name == "say" || cmd.Name == "edit" || cmd.Name == "inspectmsg" {
				deleteGuildID = "" // DM専用はグローバル登録なので空にする
			}

			err := dg.ApplicationCommandDelete(dg.State.User.ID, deleteGuildID, cmd.ID)
			if err != nil {
				fmt.Printf("コマンド削除エラー: %v\n", err)
			}
		}
	}
}
