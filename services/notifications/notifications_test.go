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
