package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/bwmarrin/discordgo"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

const (
	defaultCalendarSyncIntervalMinutes = 60
	defaultCalendarLookaheadDays       = 90
	defaultFirestoreCollection         = "calendar_sync_events"
	jstOffsetSeconds                   = 9 * 60 * 60
)

var jstLocation = time.FixedZone("JST", jstOffsetSeconds)

type calendarSyncConfig struct {
	Enabled    bool
	ProjectID  string
	GuildID    string
	Interval   time.Duration
	Lookahead  time.Duration
	Collection string
	AnnounceTZ *time.Location
	AnnounceHr int
	Targets    []calendarSyncTarget
}

type calendarSyncTarget struct {
	Name              string
	CalendarID        string
	AnnounceChannelID string
}

func loadCalendarSyncConfig() calendarSyncConfig {
	intervalMins := envInt("CAL_SYNC_INTERVAL_MINUTES", defaultCalendarSyncIntervalMinutes)
	if intervalMins < 1 {
		intervalMins = defaultCalendarSyncIntervalMinutes
	}
	lookaheadDays := envInt("CAL_SYNC_LOOKAHEAD_DAYS", defaultCalendarLookaheadDays)
	if lookaheadDays < 1 {
		lookaheadDays = defaultCalendarLookaheadDays
	}

	guildID := strings.TrimSpace(os.Getenv("DISCORD_GUILD_ID"))
	if guildID == "" {
		guildID = strings.TrimSpace(os.Getenv("ALLOWED_GUILD_ID"))
	}

	collection := strings.TrimSpace(os.Getenv("FIRESTORE_COLLECTION_CALENDAR_SYNC"))
	if collection == "" {
		collection = defaultFirestoreCollection
	}

	announceTZName := strings.TrimSpace(os.Getenv("CAL_SYNC_ANNOUNCE_TIMEZONE"))
	if announceTZName == "" {
		announceTZName = "Asia/Tokyo"
	}
	announceTZ, err := time.LoadLocation(announceTZName)
	if err != nil {
		announceTZ = jstLocation
	}
	announceHour := envInt("CAL_SYNC_ANNOUNCE_HOUR", 9)
	if announceHour < 0 || announceHour > 23 {
		announceHour = 9
	}

	defaultCalendarID := normalizeCalendarID(strings.TrimSpace(os.Getenv("GOOGLE_CALENDAR_ID")))
	defaultAnnounceChannelID := strings.TrimSpace(os.Getenv("DISCORD_EVENT_ANNOUNCE_CHANNEL_ID"))

	targets := buildCalendarTargets(defaultCalendarID, defaultAnnounceChannelID)

	return calendarSyncConfig{
		Enabled:    envBool("ENABLE_CALENDAR_SYNC", false),
		ProjectID:  loadCalendarProjectID(),
		GuildID:    guildID,
		Interval:   time.Duration(intervalMins) * time.Minute,
		Lookahead:  time.Duration(lookaheadDays) * 24 * time.Hour,
		Collection: collection,
		AnnounceTZ: announceTZ,
		AnnounceHr: announceHour,
		Targets:    targets,
	}
}

func startCalendarSyncWorker(ctx context.Context, dg *discordgo.Session) error {
	cfg := loadCalendarSyncConfig()
	if !cfg.Enabled {
		return nil
	}
	if cfg.ProjectID == "" || cfg.GuildID == "" || len(cfg.Targets) == 0 {
		return fmt.Errorf("calendar sync: missing required env vars (FIREBASE_PROJECT_ID or GOOGLE_SERVICE_ACCOUNT_JSON.project_id, DISCORD_GUILD_ID/ALLOWED_GUILD_ID, GOOGLE_CALENDAR_ID or GOOGLE_CALENDAR_ID_PLAY/CREATE)")
	}

	for _, target := range cfg.Targets {
		if target.AnnounceChannelID == "" {
			return fmt.Errorf("calendar sync: missing announce channel for target %s", target.Name)
		}
	}

	googleClientOptions := buildGoogleClientOptions()

	firestoreClient, err := firestore.NewClient(ctx, cfg.ProjectID, googleClientOptions...)
	if err != nil {
		return fmt.Errorf("calendar sync: failed to create firestore client: %w", err)
	}
	store := newFirestoreCalendarStore(firestoreClient, cfg.Collection)

	calendarSvc, err := newCalendarService(ctx, googleClientOptions...)
	if err != nil {
		_ = store.Close()
		return err
	}

	go func() {
		defer func() {
			if err := store.Close(); err != nil {
			}
		}()

		run := func() {
			for _, target := range cfg.Targets {
				if err := syncCalendarTargetOnce(ctx, dg, calendarSvc, store, cfg, target); err != nil {
				}
			}
		}

		// 起動直後の初回のみ一方向同期（Google -> Discord/Firestore）
		run()
		ticker := time.NewTicker(cfg.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
	return nil
}

func newCalendarService(ctx context.Context, opts ...option.ClientOption) (*calendar.Service, error) {
	opts = append(opts, option.WithScopes(calendar.CalendarScope))
	svc, err := calendar.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("calendar sync: failed to create google calendar service: %w", err)
	}
	return svc, nil
}

func buildGoogleClientOptions() []option.ClientOption {
	jsonCred := strings.TrimSpace(os.Getenv("GOOGLE_SERVICE_ACCOUNT_JSON"))
	if jsonCred == "" {
		return nil
	}
	return []option.ClientOption{option.WithCredentialsJSON([]byte(jsonCred))}
}

func loadCalendarProjectID() string {
	if projectID := strings.TrimSpace(os.Getenv("FIREBASE_PROJECT_ID")); projectID != "" {
		return projectID
	}
	if projectID := strings.TrimSpace(os.Getenv("GOOGLE_CLOUD_PROJECT")); projectID != "" {
		return projectID
	}

	jsonCred := strings.TrimSpace(os.Getenv("GOOGLE_SERVICE_ACCOUNT_JSON"))
	if jsonCred == "" {
		return ""
	}

	var cred struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal([]byte(jsonCred), &cred); err != nil {
		return ""
	}
	return strings.TrimSpace(cred.ProjectID)
}

func fetchDiscordEventMap(dg *discordgo.Session, guildID string) (map[string]*discordgo.GuildScheduledEvent, error) {
	events, err := dg.GuildScheduledEvents(guildID, false)
	if err != nil {
		return nil, err
	}
	m := make(map[string]*discordgo.GuildScheduledEvent, len(events))
	for _, ev := range events {
		m[ev.ID] = ev
	}
	return m, nil
}

func syncCalendarTargetOnce(ctx context.Context, dg *discordgo.Session, calendarSvc *calendar.Service, store *firestoreCalendarStore, cfg calendarSyncConfig, target calendarSyncTarget) error {
	now := time.Now().UTC()
	listCall := calendarSvc.Events.List(target.CalendarID).
		SingleEvents(true).
		ShowDeleted(true).
		OrderBy("startTime").
		TimeMin(now.Format(time.RFC3339)).
		TimeMax(now.Add(cfg.Lookahead).Format(time.RFC3339))

	events, err := listCall.Do()
	if err != nil {
		return fmt.Errorf("calendar sync: failed to fetch calendar events: %w", err)
	}

	discordEventMap, err := fetchDiscordEventMap(dg, cfg.GuildID)
	if err != nil {
		return fmt.Errorf("calendar sync: failed to fetch discord events: %w", err)
	}

	for _, ev := range events.Items {
		if ev == nil || strings.TrimSpace(ev.Id) == "" {
			continue
		}

		if ev.Status == "cancelled" {
			continue
		}

		startAt, endAt, err := extractCalendarEventTime(ev)
		if err != nil {
			continue
		}

		if err := syncActiveEvent(ctx, dg, calendarSvc, store, cfg, target, ev, startAt, endAt, discordEventMap); err != nil {
			continue
		}

		if isAnnouncementWindow(now, cfg) {
			if err := maybeAnnounceByTargetRule(ctx, dg, calendarSvc, store, cfg, target, ev, now, startAt); err != nil {
			}
		}
	}

	return nil
}

func syncActiveEvent(ctx context.Context, dg *discordgo.Session, calendarSvc *calendar.Service, store *firestoreCalendarStore, cfg calendarSyncConfig, target calendarSyncTarget, ev *calendar.Event, startAt time.Time, endAt time.Time, discordEventMap map[string]*discordgo.GuildScheduledEvent) error {
	rec, err := store.Get(ctx, target.CalendarID, ev.Id)
	if err != nil {
		return err
	}

	var discordEv *discordgo.GuildScheduledEvent
	if rec != nil && rec.DiscordEventID != "" {
		if de, ok := discordEventMap[rec.DiscordEventID]; ok {
			if de.Status != discordgo.GuildScheduledEventStatusCompleted &&
				de.Status != discordgo.GuildScheduledEventStatusCanceled {
				discordEv = de
			}
		}
	}

	params := buildDiscordScheduledEventParams(target, ev, startAt, endAt)
	now := time.Now().UTC()
	googleUpdatedAt := parseGoogleUpdated(ev.Updated)

	if discordEv == nil || rec == nil {
		created, err := dg.GuildScheduledEventCreate(cfg.GuildID, params)
		if err != nil {
			return err
		}
		var announced5d bool
		var announced5dAt time.Time
		if rec != nil {
			announced5d = rec.Announced5d
			announced5dAt = rec.Announced5dAt
		}
		return store.Upsert(ctx, calendarSyncRecord{
			GoogleEventID:    ev.Id,
			DiscordEventID:   created.ID,
			CalendarID:       target.CalendarID,
			Title:            strings.TrimSpace(ev.Summary),
			StartAt:          startAt,
			EndAt:            endAt,
			Location:         strings.TrimSpace(ev.Location),
			Description:      strings.TrimSpace(ev.Description),
			GoogleUpdatedAt:  googleUpdatedAt,
			GoogleEtag:       ev.Etag,
			Announced5d:      announced5d,
			Announced5dAt:    announced5dAt,
			LastSyncedAt:     now,
			Status:           "active",
			GoogleEventLink:  ev.HtmlLink,
			DiscordEventLink: buildDiscordEventURL(cfg.GuildID, created.ID),
		})
	}

	if discordEventNeedsUpdate(discordEv, ev, startAt, endAt) {
		if _, err := dg.GuildScheduledEventEdit(cfg.GuildID, discordEv.ID, params); err != nil {
			return err
		}
	}

	return store.Upsert(ctx, calendarSyncRecord{
		GoogleEventID:    ev.Id,
		DiscordEventID:   discordEv.ID,
		CalendarID:       target.CalendarID,
		Title:            strings.TrimSpace(ev.Summary),
		StartAt:          startAt,
		EndAt:            endAt,
		Location:         strings.TrimSpace(ev.Location),
		Description:      strings.TrimSpace(ev.Description),
		GoogleUpdatedAt:  googleUpdatedAt,
		GoogleEtag:       ev.Etag,
		Announced5d:      rec.Announced5d,
		Announced5dAt:    rec.Announced5dAt,
		LastSyncedAt:     now,
		Status:           "active",
		GoogleEventLink:  ev.HtmlLink,
		DiscordEventLink: buildDiscordEventURL(cfg.GuildID, discordEv.ID),
	})
}

func maybeAnnounceByTargetRule(ctx context.Context, dg *discordgo.Session, calendarSvc *calendar.Service, store *firestoreCalendarStore, cfg calendarSyncConfig, target calendarSyncTarget, ev *calendar.Event, now time.Time, startAt time.Time) error {
	windowDays := announceWindowDaysByTarget(target)
	if !shouldAnnounceWithinDays(now, startAt, windowDays) {
		return nil
	}
	previousEnded, err := hasPreviousEventEnded(ctx, calendarSvc, target, ev.Id, startAt, now)
	if err != nil {
		return err
	}
	if !previousEnded {
		return nil
	}

	should, err := store.TryMarkAnnounced5d(ctx, target.CalendarID, ev.Id)
	if err != nil {
		return err
	}
	if !should {
		return nil
	}

	msg := buildAnnounceMessage(target, ev, startAt)
	if _, err := dg.ChannelMessageSend(target.AnnounceChannelID, msg); err != nil {
		return err
	}
	return nil
}

func announceWindowDaysByTarget(target calendarSyncTarget) int {
	name := strings.ToUpper(strings.TrimSpace(target.Name))
	switch name {
	case "CREATE":
		return 7
	case "PLAY":
		return 5
	default:
		return 5
	}
}

func shouldAnnounceWithinDays(now time.Time, startAt time.Time, days int) bool {
	if days < 1 {
		return false
	}
	remaining := startAt.Sub(now)
	return remaining > 0 && remaining <= time.Duration(days)*24*time.Hour
}

func isAnnouncementWindow(now time.Time, cfg calendarSyncConfig) bool {
	localNow := now.In(cfg.AnnounceTZ)
	return localNow.Hour() == cfg.AnnounceHr
}

func buildAnnounceMessage(target calendarSyncTarget, ev *calendar.Event, startAt time.Time) string {
	description := strings.TrimSpace(ev.Description)
	title := safeEventSummary(ev.Summary)
	location := strings.TrimSpace(ev.Location)
	startStr := formatJapaneseDateTime(startAt)

	roleID, _ := getRoleIDAndCategoryName(target.Name)
	if roleID == "" {
		return fmt.Sprintf("📣 **%s**\n日時: %s\n場所: %s\n\n%s\n\n参加する方はリアクションをお願いします！", title, startStr, location, description)
	}

	var message string
	if title == "プレイ会" || title == "制作会" {
		message = fmt.Sprintf("<@&%s>\n", roleID)
		message += fmt.Sprintf("### 次回の%sは、%sに%sで行います！\n\n", title, startStr, location)
		message += description
		message += "\n\n参加する方はリアクションをお願いします！"
	} else {
		message = fmt.Sprintf("<@&%s>\n", roleID)
		message += fmt.Sprintf("## %sのお知らせ\n", title)
		message += fmt.Sprintf("以下の通り、%sを開催します！\n", title)
		message += fmt.Sprintf("日時：%s\n", startStr)
		if location != "" {
			message += fmt.Sprintf("場所：%s\n", location)
		}
		message += fmt.Sprintf("\n%s", description)
		message += "\n\n参加する方はリアクションをお願いします！"
	}

	return message
}

func formatJapaneseDateTime(t time.Time) string {
	tJST := t.In(jstLocation)
	weekDays := []string{"日", "月", "火", "水", "木", "金", "土"}
	dayStr := weekDays[tJST.Weekday()]
	return fmt.Sprintf("%d月%d日(%s) %02d:%02d", tJST.Month(), tJST.Day(), dayStr, tJST.Hour(), tJST.Minute())
}

func getRoleIDAndCategoryName(targetName string) (roleID string, categoryName string) {
	name := strings.ToUpper(strings.TrimSpace(targetName))
	if name == "PLAY" {
		return "1140643986084728883", "プレイ会"
	} else if name == "CREATE" {
		return "1140645113467523087", "制作会"
	}
	return "", ""
}

func hasPreviousEventEnded(ctx context.Context, calendarSvc *calendar.Service, target calendarSyncTarget, currentEventID string, currentStart time.Time, now time.Time) (bool, error) {
	call := calendarSvc.Events.List(target.CalendarID).
		SingleEvents(true).
		ShowDeleted(false).
		OrderBy("startTime").
		TimeMax(currentStart.Format(time.RFC3339)).
		MaxResults(2500)

	var latestEnd time.Time
	for {
		page, err := call.Context(ctx).Do()
		if err != nil {
			return false, err
		}
		for _, item := range page.Items {
			if item == nil || strings.TrimSpace(item.Id) == "" || item.Id == currentEventID {
				continue
			}
			startAt, endAt, err := extractCalendarEventTime(item)
			if err != nil {
				continue
			}
			if !startAt.Before(currentStart) {
				continue
			}
			if latestEnd.IsZero() || endAt.After(latestEnd) {
				latestEnd = endAt
			}
		}
		if page.NextPageToken == "" {
			break
		}
		call.PageToken(page.NextPageToken)
	}

	if latestEnd.IsZero() {
		return false, nil
	}
	return !latestEnd.After(now), nil
}

func buildDiscordScheduledEventParams(_ calendarSyncTarget, ev *calendar.Event, startAt time.Time, endAt time.Time) *discordgo.GuildScheduledEventParams {
	desc := strings.TrimSpace(ev.Description)
	location := strings.TrimSpace(ev.Location)

	name := safeEventSummary(ev.Summary)
	return &discordgo.GuildScheduledEventParams{
		Name:               name,
		Description:        trimToDiscordDescription(desc),
		PrivacyLevel:       discordgo.GuildScheduledEventPrivacyLevelGuildOnly,
		ScheduledStartTime: &startAt,
		ScheduledEndTime:   &endAt,
		EntityType:         discordgo.GuildScheduledEventEntityTypeExternal,
		EntityMetadata: &discordgo.GuildScheduledEventEntityMetadata{
			Location: trimToDiscordLocation(location),
		},
	}
}

func buildCalendarTargets(defaultCalendarID string, defaultAnnounceChannelID string) []calendarSyncTarget {
	targets := make([]calendarSyncTarget, 0, 3)
	seen := map[string]struct{}{}

	add := func(name string, calendarID string, announceChannelID string) {
		calendarID = normalizeCalendarID(calendarID)
		announceChannelID = strings.TrimSpace(announceChannelID)
		if calendarID == "" {
			return
		}
		if announceChannelID == "" {
			announceChannelID = defaultAnnounceChannelID
		}
		key := name + "|" + calendarID + "|" + announceChannelID
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		targets = append(targets, calendarSyncTarget{Name: name, CalendarID: calendarID, AnnounceChannelID: announceChannelID})
	}

	add("DEFAULT", defaultCalendarID, defaultAnnounceChannelID)
	add("PLAY", os.Getenv("GOOGLE_CALENDAR_ID_PLAY"), os.Getenv("DISCORD_EVENT_ANNOUNCE_CHANNEL_ID_PLAY"))
	add("CREATE", os.Getenv("GOOGLE_CALENDAR_ID_CREATE"), os.Getenv("DISCORD_EVENT_ANNOUNCE_CHANNEL_ID_CREATE"))

	return targets
}

func normalizeCalendarID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	parsed, err := url.Parse(raw)
	if err == nil {
		src := strings.TrimSpace(parsed.Query().Get("src"))
		if src != "" {
			decoded, decErr := url.QueryUnescape(src)
			if decErr == nil {
				return decoded
			}
			return src
		}
	}

	decoded, err := url.QueryUnescape(raw)
	if err == nil {
		return decoded
	}
	return raw
}

func extractCalendarEventTime(ev *calendar.Event) (time.Time, time.Time, error) {
	if ev.Start == nil || ev.End == nil {
		return time.Time{}, time.Time{}, fmt.Errorf("missing start or end")
	}

	if ev.Start.DateTime != "" && ev.End.DateTime != "" {
		startAt, err := time.Parse(time.RFC3339, ev.Start.DateTime)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		endAt, err := time.Parse(time.RFC3339, ev.End.DateTime)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		if !endAt.After(startAt) {
			endAt = startAt.Add(1 * time.Hour)
		}
		return startAt.UTC(), endAt.UTC(), nil
	}

	if ev.Start.Date != "" && ev.End.Date != "" {
		loc := jstLocation
		startDate, err := time.ParseInLocation("2006-01-02", ev.Start.Date, loc)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		endDate, err := time.ParseInLocation("2006-01-02", ev.End.Date, loc)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		if !endDate.After(startDate) {
			endDate = startDate.Add(24 * time.Hour)
		}
		return startDate.UTC(), endDate.UTC(), nil
	}

	return time.Time{}, time.Time{}, fmt.Errorf("unsupported event date format")
}

func discordEventNeedsUpdate(discordEv *discordgo.GuildScheduledEvent, calEv *calendar.Event, startAt, endAt time.Time) bool {
	if strings.TrimSpace(calEv.Summary) != discordEv.Name {
		return true
	}
	if strings.TrimSpace(calEv.Description) != strings.TrimSpace(discordEv.Description) {
		return true
	}
	if !discordEv.ScheduledStartTime.Equal(startAt) {
		return true
	}
	if discordEv.ScheduledEndTime == nil || !discordEv.ScheduledEndTime.Equal(endAt) {
		return true
	}
	wantLoc := trimToDiscordLocation(strings.TrimSpace(calEv.Location))
	if wantLoc != discordEv.EntityMetadata.Location {
		return true
	}
	return false
}

func buildDiscordEventURL(guildID string, eventID string) string {
	if guildID == "" || eventID == "" {
		return ""
	}
	return fmt.Sprintf("https://discord.com/events/%s/%s", guildID, eventID)
}

func parseGoogleUpdated(s string) time.Time {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
	if err != nil {
		return time.Now().UTC()
	}
	return t.UTC()
}

func safeEventSummary(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "（タイトル未設定）"
	}
	return s
}

func trimToDiscordDescription(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 1000 {
		return s
	}
	return s[:1000]
}

func trimToDiscordLocation(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 100 {
		return s
	}
	return s[:100]
}

func envBool(key string, def bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func envInt(key string, def int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return v
}
