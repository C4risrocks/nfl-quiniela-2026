package notifications

import (
	"os"
	"testing"
	"time"

	"nfl-quiniela-2026/db"
)

func TestReminderWorkerLogic(t *testing.T) {
	os.Remove("test_notifications.db")
	database, err := db.InitDB("sqlite", "test_notifications.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer database.Close()
	defer os.Remove("test_notifications.db")

	repo := db.NewRepository(database)
	_ = db.SeedDatabase(repo, "admin", "admin@test.com", "pass123", 2026)

	season, _ := repo.GetActiveSeason(2026)
	week, _ := repo.GetWeekByNumber(season.ID, 1)

	// Create 2 test users
	u1, _ := repo.CreateUser("user1", "user1@test.com", "hash", "player")
	u2, _ := repo.CreateUser("user2", "user2@test.com", "hash", "player")

	// Verify both have pending picks
	pending, err := repo.GetUsersWithPendingPicks(week.ID)
	if err != nil {
		t.Fatalf("GetUsersWithPendingPicks: %v", err)
	}

	foundU1, foundU2 := false, false
	for _, p := range pending {
		if p.ID == u1.ID {
			foundU1 = true
		}
		if p.ID == u2.ID {
			foundU2 = true
		}
	}
	if !foundU1 || !foundU2 {
		t.Errorf("Expected u1 and u2 to have pending picks, got %d users", len(pending))
	}

	// Test EmailSender in Mock mode (empty host)
	sender := NewEmailSender("", 587, "", "", "quiniela@test.com", "http://localhost:8080")
	worker := NewReminderWorker(repo, sender, 2026)

	// Test Manual Reminder
	sent, err := worker.SendManualReminders(week.ID)
	if err != nil {
		t.Fatalf("SendManualReminders: %v", err)
	}
	if sent < 2 {
		t.Errorf("Expected at least 2 reminders sent, got %d", sent)
	}

	// Test Duplicate Tracking
	_ = repo.LogReminderSent(u1.ID, week.ID, "24h")
	hasSent, err := repo.HasUserReceivedReminder(u1.ID, week.ID, "24h")
	if err != nil || !hasSent {
		t.Errorf("Expected u1 to have received 24h reminder, got %v", hasSent)
	}
	hasSent2, _ := repo.HasUserReceivedReminder(u2.ID, week.ID, "24h")
	if hasSent2 {
		t.Error("Expected u2 to not have received 24h reminder yet")
	}

	_ = time.Now()
}

func TestWeeklyRecapEmailAndDigest(t *testing.T) {
	dbPath := "test_recap_unit.db"
	_ = os.Remove(dbPath)
	defer os.Remove(dbPath)

	database, err := db.InitDB("sqlite", dbPath)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer database.Close()

	repo := db.NewRepository(database)
	_ = db.SeedDatabase(repo, "admin", "admin@test.com", "pass123", 2026)

	season, _ := repo.GetActiveSeason(2026)
	week, _ := repo.GetWeekByNumber(season.ID, 1)

	user, _ := repo.CreateUser("recap_player", "player@test.com", "hash", "player")

	sender := NewEmailSender("", 587, "", "", "quiniela@test.com", "http://localhost:8080")
	worker := NewReminderWorker(repo, sender, 2026)

	// 1. Test SendWeeklyRecapEmail with mock data
	recapData := &db.WeeklyRecapData{
		WeekID:            week.ID,
		WeekNumber:        1,
		WeekName:          "Semana 1",
		TotalParticipants: 10,
		Podium: []*db.PodiumEntry{
			{Rank: 1, UserID: user.ID, Username: "recap_player", TotalPoints: 12, CorrectPicks: 10, TotalPicks: 16},
		},
		UserRecap: &db.UserRecapStats{
			WeeklyRank:      1,
			TotalPoints:     12,
			CorrectPicks:    10,
			TotalGames:      16,
			AccuracyPercent: 63,
			SeasonRank:      1,
			SeasonTotalPts:  12,
		},
		NextWeekNumber: 2,
		NextWeekName:   "Semana 2",
	}

	if err := sender.SendWeeklyRecapEmail(user, recapData); err != nil {
		t.Fatalf("SendWeeklyRecapEmail failed: %v", err)
	}

	// 2. Test SendManualWeeklyRecap
	sent, err := worker.SendManualWeeklyRecap(week.ID)
	if err != nil {
		t.Fatalf("SendManualWeeklyRecap failed: %v", err)
	}
	if sent < 1 {
		t.Errorf("Expected at least 1 recap sent, got %d", sent)
	}

	// 3. Verify user received in-app notification
	notifs, err := repo.ListUserNotifications(user.ID, 5)
	if err != nil || len(notifs) == 0 {
		t.Fatalf("Expected in-app notification for user, got err: %v, count: %d", err, len(notifs))
	}
	if notifs[0].Type != "weekly_recap" {
		t.Errorf("Expected notif type 'weekly_recap', got %s", notifs[0].Type)
	}
}

