package notifications

import (
	"context"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
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
	dbPath := t.TempDir() + "/push.db"
	_ = os.Remove(dbPath)
	defer os.Remove(dbPath)

	database, err := db.InitDB("sqlite", dbPath)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer database.Close()

	repo := db.NewRepository(database)
	if err := db.SeedDatabase(repo, "admin", "admin@test.com", "pass123", 2026); err != nil {
		t.Fatal(err)
	}

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

	user, err := repo.CreateUser("pusher", "pusher@test.com", "hash", "player")
	if err != nil {
		t.Fatal(err)
	}

	// 2. Mock Push Gateway server
	var statusCode int = http.StatusCreated
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
	}))
	defer ts.Close()

	pushSvc = testPushService(t, repo, pubKey, privKey, ts)

	// Direct endpoint to mock server
	sub1 := &db.PushSubscription{
		UserID:   user.ID,
		Endpoint: "https://example.com/fcm/sub1",
		// Valid base64url keys so webpush-go encryption doesn't fail before network
		P256dh:    pubKey,
		Auth:      "kZ1q9zP3p4_x5y6w7v8u9g",
		UserAgent: "TestAgent/1.0",
	}
	if err := repo.SavePushSubscription(sub1); err != nil {
		t.Fatal(err)
	}

	// 3. Test successful dispatch
	statusCode = http.StatusCreated
	err = pushSvc.SendNotification(context.Background(), sub1, PushPayload{
		Title: "Test Title",
		Body:  "Test Body",
		URL:   "/picks",
	})
	if err != nil {
		t.Fatalf("successful dispatch: %v", err)
	}

	// 4. Test 410 Gone / 404 Not Found pruning
	statusCode = http.StatusGone
	err = pushSvc.SendNotification(context.Background(), sub1, PushPayload{
		Title: "Test Expired",
		Body:  "Expired",
	})

	if err == nil || !strings.Contains(err.Error(), "subscription expired") {
		t.Fatalf("expected expiration, got %v", err)
	}

	// Verify endpoint was automatically pruned from DB
	subsAfter, err := repo.GetUserPushSubscriptions(user.ID)
	if err != nil {
		t.Fatal(err)
	}
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

func TestValidatePushEndpoint_SSRFPrevention(t *testing.T) {

	blockedEndpoints := []string{
		"http://127.0.0.1:8080/push",
		"http://localhost:3000/api",
		"https://127.0.0.1:8443/push",
		"https://localhost/push",
		"https://169.254.169.254/latest/meta-data",
		"https://10.0.0.1/push",
		"https://192.168.1.1/push",
		"https://172.16.0.1/push",
		"ftp://example.com/push",
		"file:///etc/passwd",
	}

	for _, ep := range blockedEndpoints {
		if err := ValidatePushEndpoint(ep); err == nil {
			t.Errorf("Expected endpoint %s to be blocked by SSRF filter, but it was allowed", ep)
		}
	}

	validEndpoints := []string{
		"https://fcm.googleapis.com/fcm/send/xyz123",
		"https://updates.push.services.mozilla.com/wpush/v2/abc456",
		"https://web.push.apple.com/push/device789",
	}

	for _, ep := range validEndpoints {
		if err := validatePushEndpoint(context.Background(), ep, func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("8.8.8.8")}, nil }); err != nil {
			t.Errorf("Expected endpoint %s to be allowed, but got error: %v", ep, err)
		}
	}
}

// The resolver reports a public destination; only this per-instance connector routes
// that validated address to the TLS fixture. Production policy remains active.
func testPushService(t *testing.T, repo *db.Repository, pub, priv string, server *httptest.Server) *PushService {
	t.Helper()
	n := pushNetwork{
		lookupIP: func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("8.8.8.8")}, nil },
		dialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if addr != "8.8.8.8:443" {
				t.Errorf("unexpected unvalidated dial %s", addr)
			}
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		},
	}
	ps := newPushService(repo, pub, priv, "mailto:test@example.com", n)
	tr := ps.httpClient.Transport.(*http.Transport)
	tr.TLSClientConfig = server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	tr.TLSClientConfig.InsecureSkipVerify = false
	tr.TLSClientConfig.RootCAs = x509.NewCertPool()
	tr.TLSClientConfig.RootCAs.AddCert(server.Certificate())
	t.Cleanup(tr.CloseIdleConnections)
	return ps
}

func TestPushService_AntiRedirectAndAntiRebindingSSRF(t *testing.T) {
	for _, code := range []int{302, 307, 308} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			var initial, target atomic.Int32
			internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { target.Add(1); w.WriteHeader(201) }))
			defer internal.Close()
			gateway := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { initial.Add(1); http.Redirect(w, r, internal.URL, code) }))
			defer gateway.Close()
			ps := testPushService(t, nil, "", "", gateway)
			req, err := http.NewRequest("POST", "https://example.com/push", strings.NewReader("payload"))
			if err != nil {
				t.Fatal(err)
			}
			resp, err := ps.httpClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != code || initial.Load() != 1 || target.Load() != 0 {
				t.Fatalf("status=%d initial=%d target=%d", resp.StatusCode, initial.Load(), target.Load())
			}
		})
	}
}

func TestPushNetworkBlocksResolvedAddresses(t *testing.T) {
	for _, addresses := range [][]string{{"127.0.0.1"}, {"::1"}, {"::ffff:127.0.0.1"}, {"10.0.0.1"}, {"169.254.169.254"}, {"fc00::1"}, {"8.8.8.8", "192.168.1.1"}, {}} {
		t.Run(strings.Join(addresses, ","), func(t *testing.T) {
			var dialed atomic.Int32
			n := pushNetwork{lookupIP: func(context.Context, string) ([]net.IP, error) {
				var ips []net.IP
				for _, v := range addresses {
					ips = append(ips, net.ParseIP(v))
				}
				return ips, nil
			}, dialContext: func(context.Context, string, string) (net.Conn, error) {
				dialed.Add(1)
				return nil, fmt.Errorf("must not dial")
			}}
			ps := newPushService(nil, "", "", "", n)
			_, err := ps.httpClient.Get("https://example.com/push")
			if err == nil || dialed.Load() != 0 {
				t.Fatalf("err=%v dials=%d", err, dialed.Load())
			}
		})
	}
}

func TestPushDNSRebindingAndAmbientProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9")
	var lookups, dials atomic.Int32
	n := pushNetwork{lookupIP: func(context.Context, string) ([]net.IP, error) {
		if lookups.Add(1) == 1 {
			return []net.IP{net.ParseIP("8.8.8.8")}, nil
		}
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}, dialContext: func(context.Context, string, string) (net.Conn, error) {
		dials.Add(1)
		return nil, fmt.Errorf("must not dial")
	}}
	if err := validatePushEndpoint(context.Background(), "https://example.com/push", n.lookupIP); err != nil {
		t.Fatal(err)
	}
	ps := newPushService(nil, "", "", "", n)
	if ps.httpClient.Transport.(*http.Transport).Proxy != nil {
		t.Fatal("push inherits ambient proxy")
	}
	_, err := ps.httpClient.Get("https://example.com/push")
	if err == nil || lookups.Load() != 2 || dials.Load() != 0 {
		t.Fatalf("err=%v lookups=%d dials=%d", err, lookups.Load(), dials.Load())
	}
}
