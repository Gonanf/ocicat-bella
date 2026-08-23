package store

import (
	"context"
	"testing"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/model"
)

func TestMemStore_CRUD(t *testing.T) {
	s := NewMemStore()
	ctx := context.Background()

	// 1. Create and Get User
	user := &model.User{
		ID:    "usr_test",
		Name:  "Test User",
		Email: "test@example.com",
		Role:  model.RoleDocente,
	}
	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	fetchedUser, err := s.GetUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("failed to get user: %v", err)
	}
	if fetchedUser.Name != user.Name {
		t.Errorf("expected user name %s, got %s", user.Name, fetchedUser.Name)
	}

	// 2. Create and Get Session
	now := time.Now()
	session := &model.Session{
		ID:        "sess_test",
		UserID:    user.ID,
		Kind:      model.SessionKindStaff,
		CreatedAt: now,
		ExpiresAt: now.Add(24 * time.Hour),
	}
	if err := s.CreateSession(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	fetchedSession, userForSession, err := s.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if fetchedSession.ID != session.ID {
		t.Errorf("expected session id %s, got %s", session.ID, fetchedSession.ID)
	}
	if userForSession.ID != user.ID {
		t.Errorf("expected user id %s, got %s", user.ID, userForSession.ID)
	}

	// 3. Touch Session
	touchTime := now.Add(5 * time.Minute)
	if err := s.TouchSession(ctx, session.ID, touchTime); err != nil {
		t.Fatalf("failed to touch session: %v", err)
	}
	updatedSess, _, _ := s.GetSession(ctx, session.ID)
	if !updatedSess.LastSeenAt.Equal(touchTime) {
		t.Errorf("expected touch time %v, got %v", touchTime, updatedSess.LastSeenAt)
	}

	// 4. Delete Session
	if err := s.DeleteSession(ctx, session.ID); err != nil {
		t.Fatalf("failed to delete session: %v", err)
	}
	_, _, err = s.GetSession(ctx, session.ID)
	if err == nil {
		t.Errorf("expected error after delete, got nil")
	}
}
