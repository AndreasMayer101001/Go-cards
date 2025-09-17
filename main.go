package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Subscription struct {
	ID          uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	ServiceName string     `gorm:"size:255;not null" json:"service_name"`
	Price       int        `gorm:"not null" json:"price"`
	UserID      uuid.UUID  `gorm:"type:uuid;index;not null" json:"user_id"`
	StartDate   time.Time  `gorm:"type:date;not null" json:"start_date"`
	EndDate     *time.Time `gorm:"type:date;index" json:"end_date"`
	CreatedAt   time.Time  `gorm:"type:date;not null;default:CURRENT_DATE" json:"created_at"`
}

type SubscriptionCreate struct {
	ServiceName string    `json:"service_name"`
	Price       int       `json:"price"`
	UserID      uuid.UUID `json:"user_id"`
	StartDate   string    `json:"start_date"`
	EndDate     *string   `json:"end_date"`
}

type SubscriptionUpdate struct {
	ServiceName *string `json:"service_name"`
	Price       *int    `json:"price"`
	StartDate   *string `json:"start_date"`
	EndDate     *string `json:"end_date"`
}

func parseMMYYYY(value string) (time.Time, error) {
	t, err := time.Parse("01-2006", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("Неверный формат даты. Ожидается MM-YYYY: %w", err)
	}
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC), nil
}

func formatMMYYYY(t *time.Time) *string {
	if t == nil || t.IsZero() {
		return nil
	}
	s := t.Format("01-2006")
	return &s
}

func clampPeriod(start, end, subStart time.Time, subEnd *time.Time) (time.Time, time.Time, bool) {
	effectiveEnd := end
	if subEnd != nil && !subEnd.IsZero() && subEnd.Before(effectiveEnd) {
		effectiveEnd = *subEnd
	}
	overlapStart := start
	if subStart.After(start) {
		overlapStart = subStart
	}
	overlapEnd := effectiveEnd
	if end.Before(effectiveEnd) {
		overlapEnd = end
	}
	if overlapStart.After(overlapEnd) {
		return time.Time{}, time.Time{}, false
	}
	return overlapStart, overlapEnd, true
}

func countMonths(start, end time.Time) int {
	return (end.Year()-start.Year())*12 + int(end.Month()-start.Month()) + 1
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/subscriptions?sslmode=disable"
	}

	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{})
	if err != nil {
		log.Fatalf("db connect error: %v", err)
	}

	if err := db.AutoMigrate(&Subscription{}); err != nil {
		log.Fatalf("migrate error: %v", err)
	}

	r := chi.NewRouter()

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/subscriptions", func(w http.ResponseWriter, req *http.Request) {
			var payload SubscriptionCreate
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			start, err := parseMMYYYY(payload.StartDate)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			var endPtr *time.Time
			if payload.EndDate != nil {
				e, err := parseMMYYYY(*payload.EndDate)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				endPtr = &e
			}
			m := Subscription{
				ServiceName: payload.ServiceName,
				Price:       payload.Price,
				UserID:      payload.UserID,
				StartDate:   start,
				EndDate:     endPtr,
			}
			if err := db.Create(&m).Error; err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusCreated, m)
		})

		r.Get("/subscriptions/{id}", func(w http.ResponseWriter, req *http.Request) {
			id, err := uuid.Parse(chi.URLParam(req, "id"))
			if err != nil {
				http.Error(w, "invalid id", http.StatusBadRequest)
				return
			}
			var m Subscription
			if err := db.First(&m, "id = ?", id).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					http.Error(w, "Подписка не найдена", http.StatusNotFound)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, m)
		})

		r.Get("/subscriptions", func(w http.ResponseWriter, req *http.Request) {
			var rows []Subscription
			q := db.Model(&Subscription{})
			if u := req.URL.Query().Get("user_id"); u != "" {
				if uid, err := uuid.Parse(u); err == nil {
					q = q.Where("user_id = ?", uid)
				}
			}
			if sname := req.URL.Query().Get("service_name"); sname != "" {
				q = q.Where("service_name = ?", sname)
			}
			limit := 50
			offset := 0
			if v := req.URL.Query().Get("limit"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 500 {
					limit = n
				}
			}
			if v := req.URL.Query().Get("offset"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n >= 0 {
					offset = n
				}
			}
			if err := q.Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, rows)
		})

		r.Put("/subscriptions/{id}", func(w http.ResponseWriter, req *http.Request) {
			id, err := uuid.Parse(chi.URLParam(req, "id"))
			if err != nil {
				http.Error(w, "invalid id", http.StatusBadRequest)
				return
			}
			var m Subscription
			if err := db.First(&m, "id = ?", id).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					http.Error(w, "Подписка не найдена", http.StatusNotFound)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			var payload SubscriptionUpdate
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if payload.ServiceName != nil {
				m.ServiceName = *payload.ServiceName
			}
			if payload.Price != nil {
				m.Price = *payload.Price
			}
			if payload.StartDate != nil {
				if t, err := parseMMYYYY(*payload.StartDate); err == nil {
					m.StartDate = t
				} else {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			}
			if payload.EndDate != nil {
				if *payload.EndDate == "" {
					m.EndDate = nil
				} else if t, err := parseMMYYYY(*payload.EndDate); err == nil {
					m.EndDate = &t
				} else {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			}
			if err := db.Save(&m).Error; err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, m)
		})

		r.Delete("/subscriptions/{id}", func(w http.ResponseWriter, req *http.Request) {
			id, err := uuid.Parse(chi.URLParam(req, "id"))
			if err != nil {
				http.Error(w, "invalid id", http.StatusBadRequest)
				return
			}
			if err := db.Delete(&Subscription{}, "id = ?", id).Error; err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})

		r.Get("/subscriptions/aggregate/total", func(w http.ResponseWriter, req *http.Request) {
			qs := req.URL.Query()
			periodStart := qs.Get("period_start")
			periodEnd := qs.Get("period_end")
			if periodStart == "" || periodEnd == "" {
				http.Error(w, "period_start и period_end обязательны", http.StatusBadRequest)
				return
			}
			start, err := parseMMYYYY(periodStart)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			end, err := parseMMYYYY(periodEnd)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if end.Before(start) {
				http.Error(w, "Конец периода раньше начала", http.StatusBadRequest)
				return
			}

			var rows []Subscription
			q := db.Model(&Subscription{})
			if u := qs.Get("user_id"); u != "" {
				if uid, err := uuid.Parse(u); err == nil {
					q = q.Where("user_id = ?", uid)
				}
			}
			if sname := qs.Get("service_name"); sname != "" {
				q = q.Where("service_name = ?", sname)
			}
			if err := q.Find(&rows).Error; err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			total := 0
			for _, sub := range rows {
				if s, e, ok := clampPeriod(start, end, sub.StartDate, sub.EndDate); ok {
					months := countMonths(s, e)
					total += sub.Price * months
				}
			}
			writeJSON(w, http.StatusOK, map[string]int{"total": total})
		})
	})

	log.Printf("Server starting on :8080")
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
