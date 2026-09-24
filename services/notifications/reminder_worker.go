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

// Start runs periodic reminder checks and weekly digest checks in the background
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

				if count, err := w.CheckAndSendWeeklyDigests(); err != nil {
					log.Printf("[Notifications] Warning in weekly digest run: %v", err)
				} else if count > 0 {
					log.Printf("[Notifications] Dispatched %d automated weekly recap digests.", count)
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

		isWeek1Grace := week.WeekNumber == 1 && now.Before(db.Week1GraceDeadline)

		var reminderType string
		var targetDeadline time.Time

		if isWeek1Grace {
			targetDeadline = db.Week1GraceDeadline
			timeUntilDeadline := targetDeadline.Sub(now)
			if timeUntilDeadline > 0 && timeUntilDeadline <= 2*time.Hour {
				reminderType = "grace-2h"
			} else if timeUntilDeadline > 2*time.Hour && timeUntilDeadline <= 12*time.Hour {
				reminderType = "grace-12h"
			} else {
				continue
			}
		} else {
			// Find earliest upcoming game that has not kicked off yet
			var earliestUpcoming *time.Time
			for _, g := range games {
				if g.KickoffTime.After(now) {
					if earliestUpcoming == nil || g.KickoffTime.Before(*earliestUpcoming) {
						t := g.KickoffTime
						earliestUpcoming = &t
					}
				}
			}
			if earliestUpcoming == nil {
				continue // All games in this week already kicked off
			}

			targetDeadline = *earliestUpcoming
			timeUntilKickoff := targetDeadline.Sub(now)

			// Determine if within reminder windows (24 hours or 2 hours prior to upcoming kickoff)
			if timeUntilKickoff > 0 && timeUntilKickoff <= 2*time.Hour {
				reminderType = "2h"
			} else if timeUntilKickoff > 0 && timeUntilKickoff <= 24*time.Hour {
				reminderType = "24h"
			} else {
				continue
			}
		}

		usersPending, err := w.repo.GetUsersWithPendingPicks(week.ID)
		if err != nil {
			log.Printf("[Notifications] Error getting pending users for week %d: %v", week.WeekNumber, err)
			continue
		}

		for _, user := range usersPending {
			if user.IsBot() || user.IsAdmin() {
				continue
			}
			alreadySent, err := w.repo.HasUserReceivedReminder(user.ID, week.ID, reminderType)
			if err != nil || alreadySent {
				continue
			}

			// 1. In-App Notification (always alert within the web app)
			deadlineStr := targetDeadline.Format("15:04 MST")
			loc, err := time.LoadLocation("America/Mexico_City")
			if err == nil {
				deadlineStr = targetDeadline.In(loc).Format("3:04 PM CDMX")
			}
			_, _ = w.repo.CreateInAppNotification(
				user.ID,
				fmt.Sprintf("⏰ Recordatorio: %s", week.Name),
				fmt.Sprintf("Los partidos inician pronto (%s). Tienes pronósticos pendientes de registrar.", deadlineStr),
				fmt.Sprintf("/picks?week=%d", week.WeekNumber),
				"kickoff_reminder",
			)

			// 2. Email Notification (if user has enabled email kickoff reminders)
			if user.NotifyEmail && user.NotifyKickoff {
				var sendErr error
				if isWeek1Grace {
					sendErr = w.sender.SendGracePeriodReminder(user, week, targetDeadline)
				} else {
					sendErr = w.sender.SendKickoffReminder(user, week, targetDeadline)
				}
				if sendErr != nil {
					log.Printf("[Notifications] Failed sending reminder email to %s: %v", user.Email, sendErr)
				}
			}

			_ = w.repo.LogReminderSent(user.ID, week.ID, reminderType)
			totalSent++
		}
	}

	return totalSent, nil
}

// SendManualReminders dispatches immediate reminders to all users with pending picks for a week
func (w *ReminderWorker) SendManualReminders(weekID int64, requestedType ...string) (int, error) {
	week, err := w.repo.GetWeekByID(weekID)
	if err != nil {
		return 0, fmt.Errorf("week not found: %w", err)
	}

	games, err := w.repo.ListGamesByWeek(weekID)
	if err != nil || len(games) == 0 {
		return 0, fmt.Errorf("no games found for week")
	}

	now := time.Now()
	isGrace := week.WeekNumber == 1 && now.Before(db.Week1GraceDeadline)
	if len(requestedType) > 0 && requestedType[0] == "grace_period" {
		isGrace = true
	} else if len(requestedType) > 0 && requestedType[0] == "kickoff" {
		isGrace = false
	}

	var earliestKickoff time.Time
	if isGrace {
		earliestKickoff = db.Week1GraceDeadline
	} else {
		var earliestUpcoming *time.Time
		for _, g := range games {
			if g.KickoffTime.After(now) {
				if earliestUpcoming == nil || g.KickoffTime.Before(*earliestUpcoming) {
					t := g.KickoffTime
					earliestUpcoming = &t
				}
			}
		}
		if earliestUpcoming != nil {
			earliestKickoff = *earliestUpcoming
		} else {
			earliestKickoff = games[0].KickoffTime
		}
	}

	usersPending, err := w.repo.GetUsersWithPendingPicks(weekID)
	if err != nil {
		return 0, err
	}

	sent := 0
	for _, user := range usersPending {
		if user.IsBot() {
			continue
		}

		// In-App Notification
		_, _ = w.repo.CreateInAppNotification(
			user.ID,
			fmt.Sprintf("⏰ Recordatorio: %s", week.Name),
			"Tienes pronósticos pendientes de registrar. ¡Ingresa tus selecciones antes de que inicien los partidos!",
			fmt.Sprintf("/picks?week=%d", week.WeekNumber),
			"kickoff_reminder",
		)

		// Email
		var sendErr error
		if isGrace {
			sendErr = w.sender.SendGracePeriodReminder(user, week, db.Week1GraceDeadline)
		} else {
			sendErr = w.sender.SendKickoffReminder(user, week, earliestKickoff)
		}
		if sendErr != nil {
			log.Printf("[Notifications] Manual send failed for %s: %v", user.Email, sendErr)
			continue
		}
		typeTag := "manual"
		if isGrace {
			typeTag = "manual-grace"
		}
		_ = w.repo.LogReminderSent(user.ID, weekID, fmt.Sprintf("%s-%s", typeTag, time.Now().Format("20060102-1504")))
		sent++
	}

	return sent, nil
}

// CheckAndSendWeeklyDigests checks for completed weeks and sends official recap digests to all players
func (w *ReminderWorker) CheckAndSendWeeklyDigests() (int, error) {
	season, err := w.repo.GetActiveSeason(w.seasonYear)
	if err != nil {
		return 0, err
	}

	weeks, err := w.repo.ListWeeks(season.ID)
	if err != nil {
		return 0, err
	}

	totalSent := 0
	for _, week := range weeks {
		games, err := w.repo.ListGamesByWeek(week.ID)
		if err != nil || len(games) == 0 {
			continue
		}

		allFinal := true
		for _, g := range games {
			if g.Status != "final" {
				allFinal = false
				break
			}
		}

		// Only send digests if week is fully complete
		if !allFinal && week.Status != "completed" {
			continue
		}

		users, err := w.repo.ListUsers()
		if err != nil {
			continue
		}

		for _, user := range users {
			if user.IsBot() || user.IsAdmin() {
				continue
			}

			alreadySent, err := w.repo.HasUserReceivedReminder(user.ID, week.ID, "weekly_recap")
			if err != nil || alreadySent {
				continue
			}

			recap, err := w.repo.GetWeeklyRecapData(week.ID, user.ID)
			if err != nil {
				log.Printf("[Notifications] Failed building recap for user %s, week %d: %v", user.Username, week.WeekNumber, err)
				continue
			}

			// 1. In-App Notification
			userRankText := ""
			if recap.UserRecap != nil && recap.UserRecap.WeeklyRank > 0 {
				userRankText = fmt.Sprintf(" Terminaste en el puesto #%d con %d pts.", recap.UserRecap.WeeklyRank, recap.UserRecap.TotalPoints)
			}
			_, _ = w.repo.CreateInAppNotification(
				user.ID,
				fmt.Sprintf("🏆 Resultados Oficiales: %s", week.Name),
				fmt.Sprintf("La jornada ha concluido.%s Revisa los resultados completos y el podio.", userRankText),
				fmt.Sprintf("/leaderboard?week=%d", week.WeekNumber),
				"weekly_recap",
			)

			// 2. Email Digest (if user has enabled email recap digests)
			if user.NotifyEmail && user.NotifyRecap {
				if err := w.sender.SendWeeklyRecapEmail(user, recap); err != nil {
					log.Printf("[Notifications] Failed sending recap email to %s: %v", user.Email, err)
				}
			}

			_ = w.repo.LogReminderSent(user.ID, week.ID, "weekly_recap")
			totalSent++
		}
	}

	return totalSent, nil
}

// SendManualWeeklyRecap forces immediate dispatch of the weekly recap digest for a specific week
func (w *ReminderWorker) SendManualWeeklyRecap(weekID int64) (int, error) {
	week, err := w.repo.GetWeekByID(weekID)
	if err != nil {
		return 0, fmt.Errorf("week not found: %w", err)
	}

	users, err := w.repo.ListUsers()
	if err != nil {
		return 0, err
	}

	sent := 0
	typeTag := fmt.Sprintf("manual-recap-%s", time.Now().Format("20060102-1504"))

	for _, user := range users {
		if user.IsBot() || user.IsAdmin() {
			continue
		}

		recap, err := w.repo.GetWeeklyRecapData(week.ID, user.ID)
		if err != nil {
			log.Printf("[Notifications] Failed building manual recap for user %s, week %d: %v", user.Username, week.WeekNumber, err)
			continue
		}

		// In-App Notification
		userRankText := ""
		if recap.UserRecap != nil && recap.UserRecap.WeeklyRank > 0 {
			userRankText = fmt.Sprintf(" Terminaste en el puesto #%d con %d pts.", recap.UserRecap.WeeklyRank, recap.UserRecap.TotalPoints)
		}
		_, _ = w.repo.CreateInAppNotification(
			user.ID,
			fmt.Sprintf("🏆 Resumen Semanal: %s", week.Name),
			fmt.Sprintf("Resultados calculados de la jornada.%s Revisa el podio y las posiciones.", userRankText),
			fmt.Sprintf("/leaderboard?week=%d", week.WeekNumber),
			"weekly_recap",
		)

		// Email
		if user.NotifyEmail && user.NotifyRecap {
			if err := w.sender.SendWeeklyRecapEmail(user, recap); err != nil {
				log.Printf("[Notifications] Manual recap email failed for %s: %v", user.Email, err)
				continue
			}
		}

		_ = w.repo.LogReminderSent(user.ID, week.ID, typeTag)
		sent++
	}

	return sent, nil
}

