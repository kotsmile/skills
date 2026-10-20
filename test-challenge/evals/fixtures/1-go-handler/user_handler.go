package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"example.com/app/domain"
)

type UserService interface {
	Get(ctx context.Context, id int64) (domain.User, error)
}

type userDTO struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Phone     string    `json:"phone,omitempty"`
	Country   string    `json:"country"`
	City      string    `json:"city"`
	Role      string    `json:"role"`
	Active    bool      `json:"active"`
	Verified  bool      `json:"verified"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Avatar    string    `json:"avatar_url,omitempty"`
}

func toUserDTO(u domain.User) userDTO {
	return userDTO{
		ID: u.ID, Email: u.Email, Name: u.Name, Phone: u.Phone,
		Country: u.Address.Country, City: u.Address.City, Role: string(u.Role),
		Active: u.Active, Verified: u.Verified,
		CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt, Avatar: u.AvatarURL,
	}
}

type Handler struct{ users UserService }

func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_id")
		return
	}
	u, err := h.users.Get(r.Context(), id)
	if errors.Is(err, domain.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toUserDTO(u))
}
