package api

import (
	"testing"
	"time"

	"example.com/app/domain"
)

var t0 = time.Unix(0, 0)
var t1 = time.Unix(1, 0)

func TestToUserDTO_ID(t *testing.T) {
	got := toUserDTO(domain.User{ID: int64(7)})
	if got.ID != int64(7) {
		t.Fatalf("ID: got %v", got.ID)
	}
}

func TestToUserDTO_Email(t *testing.T) {
	got := toUserDTO(domain.User{Email: "a@b.c"})
	if got.Email != "a@b.c" {
		t.Fatalf("Email: got %v", got.Email)
	}
}

func TestToUserDTO_Name(t *testing.T) {
	got := toUserDTO(domain.User{Name: "Ann"})
	if got.Name != "Ann" {
		t.Fatalf("Name: got %v", got.Name)
	}
}

func TestToUserDTO_Phone(t *testing.T) {
	got := toUserDTO(domain.User{Phone: "+100"})
	if got.Phone != "+100" {
		t.Fatalf("Phone: got %v", got.Phone)
	}
}

func TestToUserDTO_Country(t *testing.T) {
	got := toUserDTO(domain.User{Address: domain.Address{Country: "NL"}})
	if got.Country != "NL" {
		t.Fatalf("Country: got %v", got.Country)
	}
}

func TestToUserDTO_City(t *testing.T) {
	got := toUserDTO(domain.User{Address: domain.Address{City: "Utrecht"}})
	if got.City != "Utrecht" {
		t.Fatalf("City: got %v", got.City)
	}
}

func TestToUserDTO_Role(t *testing.T) {
	got := toUserDTO(domain.User{Role: domain.RoleAdmin})
	if got.Role != string(domain.RoleAdmin) {
		t.Fatalf("Role: got %v", got.Role)
	}
}

func TestToUserDTO_Active(t *testing.T) {
	got := toUserDTO(domain.User{Active: true})
	if got.Active != true {
		t.Fatalf("Active: got %v", got.Active)
	}
}

func TestToUserDTO_Verified(t *testing.T) {
	got := toUserDTO(domain.User{Verified: true})
	if got.Verified != true {
		t.Fatalf("Verified: got %v", got.Verified)
	}
}

func TestToUserDTO_CreatedAt(t *testing.T) {
	got := toUserDTO(domain.User{CreatedAt: t0})
	if got.CreatedAt != t0 {
		t.Fatalf("CreatedAt: got %v", got.CreatedAt)
	}
}

func TestToUserDTO_UpdatedAt(t *testing.T) {
	got := toUserDTO(domain.User{UpdatedAt: t1})
	if got.UpdatedAt != t1 {
		t.Fatalf("UpdatedAt: got %v", got.UpdatedAt)
	}
}

func TestToUserDTO_Avatar(t *testing.T) {
	got := toUserDTO(domain.User{AvatarURL: "https://x/a.png"})
	if got.Avatar != "https://x/a.png" {
		t.Fatalf("Avatar: got %v", got.Avatar)
	}
}
