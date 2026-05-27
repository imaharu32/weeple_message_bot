package main

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"
)

func buildMessageInspectionReport(msg *discordgo.Message) []string {
	if msg == nil {
		return []string{"検査対象メッセージが空です。"}
	}

	content := msg.Content
	trimmed := strings.TrimSpace(content)
	bidOK := isRicochetNumericBidContent(content)
	lines := []string{
		fmt.Sprintf("メッセージ検査結果 channel=%s message=%s", msg.ChannelID, msg.ID),
		fmt.Sprintf("author=%s", messageAuthorLabel(msg)),
		fmt.Sprintf("bytes=%d runes=%d", len(content), utf8.RuneCountInString(content)),
		fmt.Sprintf("trimmed_changed=%t", trimmed != content),
		fmt.Sprintf("ricochet_numeric_bid=%t", bidOK),
		"raw_quoted=",
		codeBlock(strconv.Quote(content)),
		"trimmed_quoted=",
		codeBlock(strconv.Quote(trimmed)),
		"characters:",
	}

	for index, r := range []rune(content) {
		lines = append(lines, fmt.Sprintf("%02d: %s %s %s", index, quotedRune(r), formatCodepoint(r), describeRune(r)))
	}

	if len(content) == 0 {
		lines = append(lines, "(empty message)")
	}

	return chunkDiscordLines(lines, 1800)
}

func isRicochetNumericBidContent(content string) bool {
	stepsStr := strings.TrimSpace(content)
	if stepsStr == "" {
		return false
	}
	for _, ch := range stepsStr {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func messageAuthorLabel(msg *discordgo.Message) string {
	if msg.Author == nil {
		return "unknown"
	}
	if msg.Author.Username == "" {
		return msg.Author.ID
	}
	return fmt.Sprintf("%s (%s)", msg.Author.Username, msg.Author.ID)
}

func quotedRune(r rune) string {
	return strconv.QuoteRuneToASCII(r)
}

func formatCodepoint(r rune) string {
	return fmt.Sprintf("U+%04X", r)
}

func describeRune(r rune) string {
	if name, ok := commonInvisibleRuneNames[r]; ok {
		return name
	}
	if unicode.IsControl(r) {
		return "control"
	}
	if unicode.IsSpace(r) {
		return "unicode-space"
	}
	if unicode.IsDigit(r) {
		return "digit"
	}
	if unicode.IsLetter(r) {
		return "letter"
	}
	if unicode.IsPunct(r) {
		return "punctuation"
	}
	if unicode.IsSymbol(r) {
		return "symbol"
	}
	return "other"
}

func chunkDiscordLines(lines []string, limit int) []string {
	chunks := make([]string, 0, 1)
	var builder strings.Builder
	for _, line := range lines {
		if builder.Len() == 0 {
			builder.WriteString(line)
			continue
		}
		if builder.Len()+1+len(line) > limit {
			chunks = append(chunks, builder.String())
			builder.Reset()
			builder.WriteString(line)
			continue
		}
		builder.WriteByte('\n')
		builder.WriteString(line)
	}
	if builder.Len() > 0 {
		chunks = append(chunks, builder.String())
	}
	return chunks
}

func codeBlock(content string) string {
	clean := strings.ReplaceAll(content, "```", "'''")
	return "```\n" + clean + "\n```"
}

var commonInvisibleRuneNames = map[rune]string{
	' ':      "space",
	'\t':     "tab",
	'\n':     "line-feed",
	'\r':     "carriage-return",
	'\u00A0': "no-break-space",
	'\u200B': "zero-width-space",
	'\u200C': "zero-width-non-joiner",
	'\u200D': "zero-width-joiner",
	'\u2060': "word-joiner",
	'\u3000': "ideographic-space",
}
