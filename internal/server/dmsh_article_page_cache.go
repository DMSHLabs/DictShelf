// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"sync"

	"github.com/wuweidict/wudict/internal/store"
)

// articlePageCache holds rendered article portions for one search. SQLite owns
// the temporary file. Find-in-articles match counts are independent of this cache.
const articlePageCacheMaxBytes = 128 << 20

type articlePageCache struct {
	mu       sync.Mutex
	id       string
	db       *sql.DB
	access   int64
	maxBytes int64
}

func (c *articlePageCache) resetFor(id string) {
	if !wildcardCacheIDPattern.MatchString(id) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.id == id {
		return
	}
	if c.db != nil {
		_ = c.db.Close()
		c.db = nil
	}
	c.id, c.access = id, 0
}

func (c *articlePageCache) eligible(r SearchRequest) bool {
	return wildcardCacheIDPattern.MatchString(r.CacheID) &&
		(r.CacheAction == "page" || r.CacheAction == "" && r.Limit > 0 &&
			r.Dictionary != "" && r.Dictionary != "all" && !strings.Contains(r.Dictionary, ","))
}

func (c *articlePageCache) openLocked() error {
	if c.db != nil {
		return nil
	}
	db, err := store.OpenTransientCacheDB(false)
	if err != nil {
		return err
	}
	if _, err = db.Exec(`PRAGMA auto_vacuum=FULL`); err == nil {
		_, err = db.Exec(`CREATE TABLE article_page(cache_key TEXT PRIMARY KEY, payload BLOB NOT NULL, bytes INTEGER NOT NULL, accessed INTEGER NOT NULL)`)
	}
	if err != nil {
		_ = db.Close()
		return err
	}
	c.db = db
	return nil
}

func (c *articlePageCache) serve(ctx context.Context, r SearchRequest, emit func(SearchEvent) error, load func(func(SearchEvent) error) error) error {
	limit := c.maxBytes
	if limit <= 0 {
		limit = articlePageCacheMaxBytes
	}
	key, _ := json.Marshal(struct {
		SearchRequest
		Origin string
	}{r, r.Origin})
	var cached []byte
	c.mu.Lock()
	if c.id == r.CacheID && c.openLocked() == nil {
		if c.db.QueryRowContext(ctx, `SELECT payload FROM article_page WHERE cache_key=?`, string(key)).Scan(&cached) == nil {
			c.access++
			_, _ = c.db.ExecContext(ctx, `UPDATE article_page SET accessed=? WHERE cache_key=?`, c.access, string(key))
		}
	}
	c.mu.Unlock()
	if len(cached) > 0 {
		var events []SearchEvent
		if json.Unmarshal(cached, &events) == nil {
			for _, event := range events {
				if err := emit(event); err != nil {
					return err
				}
			}
			return nil
		}
	}
	var events []SearchEvent
	err := load(func(event SearchEvent) error {
		if err := emit(event); err != nil {
			return err
		}
		events = append(events, event)
		return nil
	})
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// A growing result list can change both Total and HasNext. Re-read it
	// until its ID scan is complete instead of freezing intermediate metadata.
	if r.CacheAction == "page" && (len(events) != 1 || events[0].T != "hit" || !events[0].Complete) {
		return nil
	}
	for _, event := range events {
		if event.Error != "" || event.Skipped || event.Deferred {
			return nil
		}
	}
	payload, err := json.Marshal(events)
	if err != nil || int64(len(payload)) > limit {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.id != r.CacheID || c.openLocked() != nil {
		return nil
	}
	c.access++
	if _, err = c.db.ExecContext(ctx, `INSERT OR REPLACE INTO article_page(cache_key,payload,bytes,accessed) VALUES(?,?,?,?)`, string(key), payload, len(payload), c.access); err != nil {
		return nil
	}
	for {
		var total int64
		if c.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(bytes),0) FROM article_page`).Scan(&total) != nil || total <= limit {
			break
		}
		if _, err = c.db.ExecContext(ctx, `DELETE FROM article_page WHERE cache_key=(SELECT cache_key FROM article_page ORDER BY accessed LIMIT 1)`); err != nil {
			break
		}
	}
	return nil
}
