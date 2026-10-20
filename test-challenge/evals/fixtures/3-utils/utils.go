package utils

import (
	"strings"

	"example.com/app/domain"
)

func FilterActive(users []domain.User) []domain.User {
	var out []domain.User
	for _, u := range users {
		if u.Active {
			out = append(out, u)
		}
	}
	return out
}

func FindByID(users []domain.User, id int64) (domain.User, bool) {
	for _, u := range users {
		if u.ID == id {
			return u, true
		}
	}
	return domain.User{}, false
}

func IsEmpty(s string) bool {
	return strings.TrimSpace(s) == ""
}
