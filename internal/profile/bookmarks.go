package profile

import (
	"sync"
	"time"
)

type Bookmark struct {
	URL       string    `json:"url"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type BookmarkStore struct {
	mu        sync.Mutex
	profile   *Profile
	bookmarks []Bookmark
}

func NewBookmarkStore(p *Profile) (*BookmarkStore, error) {
	store := &BookmarkStore{
		profile:   p,
		bookmarks: []Bookmark{},
	}
	if err := p.LoadJSON("bookmarks.json", &store.bookmarks); err != nil {
		return nil, err
	}
	if store.bookmarks == nil {
		store.bookmarks = []Bookmark{}
	}

	return store, nil
}

func (s *BookmarkStore) Add(url, title string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	for i := range s.bookmarks {
		if s.bookmarks[i].URL == url {
			s.bookmarks[i].Title = title
			s.bookmarks[i].UpdatedAt = now
			return s.persist()
		}
	}

	s.bookmarks = append(s.bookmarks, Bookmark{
		URL:       url,
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
	})
	return s.persist()
}

func (s *BookmarkStore) Remove(url string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.bookmarks {
		if s.bookmarks[i].URL == url {
			s.bookmarks = append(s.bookmarks[:i], s.bookmarks[i+1:]...)
			return s.persist()
		}
	}

	return nil
}

func (s *BookmarkStore) Contains(url string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, bookmark := range s.bookmarks {
		if bookmark.URL == url {
			return true
		}
	}

	return false
}

func (s *BookmarkStore) List() []Bookmark {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]Bookmark(nil), s.bookmarks...)
}

func (s *BookmarkStore) persist() error {
	return s.profile.SaveJSON("bookmarks.json", s.bookmarks)
}
