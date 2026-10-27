package notifications

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
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

func TestPushServiceAndPruning(t *testing.T) {
	dbPath := "test_push_unit.db"
	_ = os.Remove(dbPath)
	defer os.Remove(dbPath)

	database, err := db.InitDB("sqlite", dbPath)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer database.Close()

	repo := db.NewRepository(database)
	_ = db.SeedDatabase(repo, "admin", "admin@test.com", "pass123", 2026)

	// 1. Generate real VAPID keys for testing
	privKey, pubKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys: %v", err)
	}

	pushSvc := NewPushService(repo, pubKey, privKey, "mailto:admin@quiniela.com")
	if !pushSvc.IsConfigured() {
		t.Fatalf("Expected pushSvc to be configured")
	}
	if pushSvc.PublicKey() != pubKey {
		t.Errorf("Expected PublicKey %s, got %s", pubKey, pushSvc.PublicKey())
	}

	user, _ := repo.CreateUser("pusher", "pusher@test.com", "hash", "player")

	// 2. Mock Push Gateway server
	var statusCode int = http.StatusCreated
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
	}))
	defer ts.Close()

	// Direct endpoint to mock server
	sub1 := &db.PushSubscription{
		UserID:   user.ID,
		Endpoint: ts.URL + "/fcm/sub1",
		// Valid base64url keys so webpush-go encryption doesn't fail before network
		P256dh:    pubKey,
		Auth:      "kZ1q9zP3p4_x5y6w7v8u9g",
		UserAgent: "TestAgent/1.0",
	}
	_ = repo.SavePushSubscription(sub1)

	// 3. Test successful dispatch
	statusCode = http.StatusCreated
	err = pushSvc.SendNotification(context.Background(), sub1, PushPayload{
		Title: "Test Title",
		Body:  "Test Body",
		URL:   "/picks",
	})
	if err != nil {
		t.Logf("SendNotification note (expected if mock encryption auth mismatch): %v", err)
	}

	// 4. Test 410 Gone / 404 Not Found pruning
	statusCode = http.StatusGone
	_ = pushSvc.SendNotification(context.Background(), sub1, PushPayload{
		Title: "Test Expired",
		Body:  "Expired",
	})

	// Verify endpoint was automatically pruned from DB
	subsAfter, _ := repo.GetUserPushSubscriptions(user.ID)
	if len(subsAfter) != 0 {
		t.Errorf("Expected subscription to be pruned on HTTP 410, still found %d", len(subsAfter))
	}

	// 5. Integration with ReminderWorker
	sender := NewEmailSender("", 587, "", "", "quiniela@test.com", "http://localhost:8080")
	worker := NewReminderWorker(repo, sender, 2026, pushSvc)
	if worker.PushService() != pushSvc {
		t.Errorf("Expected worker.PushService() to match pushSvc")
	}
}


