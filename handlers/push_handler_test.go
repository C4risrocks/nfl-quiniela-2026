package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"
	"nfl-quiniela-2026/db"
	"nfl-quiniela-2026/services/auth"
	"nfl-quiniela-2026/services/notifications"
)

func TestPushHandlerRoutes(t *testing.T) {
	testDB := "test_push_handler.db"
	_ = os.Remove(testDB)
	defer os.Remove(testDB)

	database, err := db.InitDB("sqlite", testDB)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer database.Close()

	repo := db.NewRepository(database)
	_ = db.SeedDatabase(repo, "admin", "admin@test.com", "pass123", 2026)

	privKey, pubKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys: %v", err)
	}

	pushSvc := notifications.NewPushService(repo, pubKey, privKey, "mailto:admin@quiniela.com")
	pushHandler := NewPushHandler(repo, pushSvc)

	// 1. Test GetVAPIDPublicKey (public)
	req := httptest.NewRequest(http.MethodGet, "/api/push/vapid-public-key", nil)
	rr := httptest.NewRecorder()
	pushHandler.GetVAPIDPublicKey(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from GetVAPIDPublicKey, got %d", rr.Code)
	}
	var pubKeyResp map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &pubKeyResp)
	if pubKeyResp["publicKey"] != pubKey {
		t.Errorf("Expected publicKey %s, got %s", pubKey, pubKeyResp["publicKey"])
	}

	// Create test user
	user, err := repo.CreateUser("pusher_web", "pusher_web@test.com", "hash", "player")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// 2. Test Subscribe endpoint
	subPayload := map[string]interface{}{
		"endpoint": "https://fcm.googleapis.com/fcm/send/test-device-endpoint",
		"keys": map[string]string{
			"p256dh": pubKey,
			"auth":   "test-auth-token-1234",
		},
	}
	bodyBytes, _ := json.Marshal(subPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/push/subscribe", bytes.NewReader(bodyBytes))
	req = req.WithContext(auth.SetUserContext(req.Context(), user))
	rr = httptest.NewRecorder()
	pushHandler.Subscribe(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from Subscribe, got %d: %s", rr.Code, rr.Body.String())
	}

	// 3. Test GetStatus endpoint
	req = httptest.NewRequest(http.MethodGet, "/api/push/status", nil)
	req = req.WithContext(auth.SetUserContext(req.Context(), user))
	rr = httptest.NewRecorder()
	pushHandler.GetStatus(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from GetStatus, got %d", rr.Code)
	}
	var statusResp map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &statusResp)
	if statusResp["deviceCount"] != float64(1) {
		t.Errorf("Expected deviceCount 1, got %v", statusResp["deviceCount"])
	}
	if statusResp["configured"] != true {
		t.Errorf("Expected configured=true, got %v", statusResp["configured"])
	}

	// 4. Test Unsubscribe endpoint
	unsubPayload := map[string]string{
		"endpoint": "https://fcm.googleapis.com/fcm/send/test-device-endpoint",
	}
	unsubBytes, _ := json.Marshal(unsubPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/push/unsubscribe", bytes.NewReader(unsubBytes))
	req = req.WithContext(auth.SetUserContext(req.Context(), user))
	rr = httptest.NewRecorder()
	pushHandler.Unsubscribe(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from Unsubscribe, got %d", rr.Code)
	}

	// Verify count is 0 after unsubscribe
	subs, _ := repo.GetUserPushSubscriptions(user.ID)
	if len(subs) != 0 {
		t.Errorf("Expected 0 subscriptions after unsubscribe, got %d", len(subs))
	}
}
