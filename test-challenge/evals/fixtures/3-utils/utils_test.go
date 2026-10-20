package utils

import (
	"testing"

	"example.com/app/domain"
)

func TestFilterActive(t *testing.T) {
	in := []domain.User{{ID: 1, Active: true}, {ID: 2}, {ID: 3, Active: true}}
	got := FilterActive(in)
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 3 {
		t.Fatalf("got %+v", got)
	}
}

func TestFilterActive_Empty(t *testing.T) {
	if got := FilterActive(nil); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestFindByID(t *testing.T) {
	u, ok := FindByID([]domain.User{{ID: 1}, {ID: 2}}, 2)
	if !ok || u.ID != 2 {
		t.Fatalf("got %+v %v", u, ok)
	}
}

func TestFindByID_Missing(t *testing.T) {
	if _, ok := FindByID([]domain.User{{ID: 1}}, 9); ok {
		t.Fatal("expected not found")
	}
}

func TestIsEmpty(t *testing.T) {
	for _, s := range []string{"", " ", "\t\n"} {
		if !IsEmpty(s) {
			t.Fatalf("%q should be empty", s)
		}
	}
	if IsEmpty("x") {
		t.Fatal("x is not empty")
	}
}
