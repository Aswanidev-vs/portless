package auth

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type MemoryStore struct {
	mu              sync.RWMutex
	usersByID       map[string]User
	userIDsByEmail  map[string]string
	userIDsByName   map[string]string
	sessionsByToken map[string]Session
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		usersByID:       make(map[string]User),
		userIDsByEmail:  make(map[string]string),
		userIDsByName:   make(map[string]string),
		sessionsByToken: make(map[string]Session),
	}
}

func (s *MemoryStore) SaveUser(_ context.Context, user User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existingID, ok := s.userIDsByEmail[user.Email]; ok && existingID != user.ID {
		return fmt.Errorf("email already exists")
	}
	if existingID, ok := s.userIDsByName[user.Username]; ok && existingID != user.ID {
		return fmt.Errorf("username already exists")
	}

	if existing, ok := s.usersByID[user.ID]; ok {
		delete(s.userIDsByEmail, existing.Email)
		delete(s.userIDsByName, existing.Username)
	}

	s.usersByID[user.ID] = user
	s.userIDsByEmail[user.Email] = user.ID
	s.userIDsByName[user.Username] = user.ID
	return nil
}

func (s *MemoryStore) GetUser(_ context.Context, id string) (User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, ok := s.usersByID[id]
	if !ok {
		return User{}, fmt.Errorf("user not found")
	}
	return user, nil
}

func (s *MemoryStore) GetUserByEmail(ctx context.Context, email string) (User, error) {
	s.mu.RLock()
	id, ok := s.userIDsByEmail[email]
	s.mu.RUnlock()
	if !ok {
		return User{}, fmt.Errorf("user not found")
	}
	return s.GetUser(ctx, id)
}

func (s *MemoryStore) GetUserByUsername(ctx context.Context, username string) (User, error) {
	s.mu.RLock()
	id, ok := s.userIDsByName[username]
	s.mu.RUnlock()
	if !ok {
		return User{}, fmt.Errorf("user not found")
	}
	return s.GetUser(ctx, id)
}

func (s *MemoryStore) ListUsers(_ context.Context, workspaceID string) ([]User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]User, 0, len(s.usersByID))
	for _, user := range s.usersByID {
		if workspaceID == "" || user.WorkspaceID == workspaceID {
			users = append(users, user)
		}
	}
	return users, nil
}

func (s *MemoryStore) DeleteUser(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.usersByID[id]
	if !ok {
		return fmt.Errorf("user not found")
	}
	delete(s.usersByID, id)
	delete(s.userIDsByEmail, user.Email)
	delete(s.userIDsByName, user.Username)
	for token, session := range s.sessionsByToken {
		if session.UserID == id {
			delete(s.sessionsByToken, token)
		}
	}
	return nil
}

func (s *MemoryStore) SaveSession(_ context.Context, session Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionsByToken[session.Token] = session
	return nil
}

func (s *MemoryStore) GetSession(_ context.Context, token string) (Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessionsByToken[token]
	if !ok {
		return Session{}, fmt.Errorf("session not found")
	}
	return session, nil
}

func (s *MemoryStore) DeleteSession(_ context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessionsByToken, token)
	return nil
}

func (s *MemoryStore) DeleteUserSessions(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, session := range s.sessionsByToken {
		if session.UserID == userID {
			delete(s.sessionsByToken, token)
		}
	}
	return nil
}

func (s *MemoryStore) CleanupExpiredSessions(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for token, session := range s.sessionsByToken {
		if !session.ExpiresAt.IsZero() && now.After(session.ExpiresAt) {
			delete(s.sessionsByToken, token)
		}
	}
	return nil
}
