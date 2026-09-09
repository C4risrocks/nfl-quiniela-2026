package notifications

import (
	"context"
	"fmt"
	"log"
	"time"

	"nfl-quiniela-2026/db"
)

type ReminderWorker struct {
	repo       *db.Repository
	sender     *EmailSender
	seasonYear int
}

func NewReminderWorker(repo *db.Repository, sender *EmailSender, seasonYear int) *ReminderWorker {
	return &ReminderWorker{
		repo:       repo,
		sender:     sender,
		seasonYear: seasonYear,
	}
}

// Start runs periodic reminder checks in the background
func (w *ReminderWorker) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		log.Printf("[Notifications] Reminder worker started (checking every %v).", interval)
		for {
			select {
			case <-ctx.Done():
				log.Println("[Notifications] Reminder worker stopped.")
				return
			case <-ticker.C:
				if count, err := w.CheckAndSendReminders(); err != nil {
					log.Printf("[Notifications] Warning in reminder run: %v", err)
				} else if count > 0 {
					log.Printf("[Notifications] Dispatched %d automated reminders.", count)
				}
			}
		}
	}()
}

// CheckAndSendReminders checks approaching kickoffs and notifies users who haven't finished picks
func (w *ReminderWorker) CheckAndSendReminders() (int, error) {
	season, err := w.repo.GetActiveSeason(w.seasonYear)
	if err != nil {
		return 0, err
	}

	weeks, err := w.repo.ListWeeks(season.ID)
	if err != nil {
		return 0, err
	}

	totalSent := 0
	now := time.Now()

	for _, week := range weeks {
		games, err := w.repo.ListGamesByWeek(week.ID)
		if err != nil || len(games) == 0 {
			continue
		}

		// Find earliest kickoff
		earliestKickoff := games[0].KickoffTime
		for _, g := range games[1:] {
			if g.KickoffTime.Before(earliestKickoff) {
				earliestKickoff = g.KickoffTime
			}
		}

		timeUntilKickoff := earliestKickoff.Sub(now)

		// Determine if within reminder windows (24 hours or 2 hours prior to kickoff)
		var reminderType string
		if timeUntilKickoff > 0 && timeUntilKickoff <= 2*time.Hour {
			reminderType = "2h"
		} else if timeUntilKickoff > 0 && timeUntilKickoff <= 24*time.Hour {
			reminderType = "24h"
		} else {
			continue
		}

		usersPending, err := w.repo.GetUsersWithPendingPicks(week.ID)
		if err != nil {
			log.Printf("[Notifications] Error getting pending users for week %d: %v", week.WeekNumber, err)
			continue
		}

		for _, user := range usersPending {
			alreadySent, err := w.repo.HasUserReceivedReminder(user.ID, week.ID, reminderType)
			if err != nil || alreadySent {
				continue
			}

			if err := w.sender.SendKickoffReminder(user, week, earliestKickoff); err != nil {
				log.Printf("[Notifications] Failed sending reminder to %s: %v", user.Email, err)
				continue
			}

			_ = w.repo.LogReminderSent(user.ID, week.ID, reminderType)
			totalSent++
		}
	}

	return totalSent, nil
}

// SendManualReminders dispatches immediate reminders to all users with pending picks for a week
func (w *ReminderWorker) SendManualReminders(weekID int64) (int, error) {
	week, err := w.repo.GetWeekByID(weekID)
	if err != nil {
		return 0, fmt.Errorf("week not found: %w", err)
	}

	games, err := w.repo.ListGamesByWeek(weekID)
	if err != nil || len(games) == 0 {
		return 0, fmt.Errorf("no games found for week")
	}

	earliestKickoff := games[0].KickoffTime
	for _, g := range games[1:] {
		if g.KickoffTime.Before(earliestKickoff) {
			earliestKickoff = g.KickoffTime
		}
	}

	usersPending, err := w.repo.GetUsersWithPendingPicks(weekID)
	if err != nil {
		return 0, err
	}

	sent := 0
	for _, user := range usersPending {
		if err := w.sender.SendKickoffReminder(user, week, earliestKickoff); err != nil {
			log.Printf("[Notifications] Manual send failed for %s: %v", user.Email, err)
			continue
		}
		_ = w.repo.LogReminderSent(user.ID, weekID, fmt.Sprintf("manual-%s", time.Now().Format("20060102-1504")))
		sent++
	}

	return sent, nil
}
