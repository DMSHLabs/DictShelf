// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/DMSHLabs/DictShelf/internal/dict"
)

// FavouriteRecord is collection metadata attached to one prepared article.
// The article body lives in text.db and its resources live in media.db.
type FavouriteRecord struct {
	EntryID        int64
	BodySize       int64
	ID             string
	SourceID       string
	SourceHeadword string
	SourceJSON     string
	AssetsJSON     string
}

type FavouriteAsset struct {
	URL   string `json:"url"`
	Name  string `json:"name"`
	MIME  string `json:"mime,omitempty"`
	Token string `json:"token"`
	Size  int64  `json:"size,omitempty"`
}

type ArticleStream struct {
	rows *sql.Rows
	text *Store
}

func (s *Store) OpenArticleStream(ctx context.Context) (*ArticleStream, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,w,m FROM entry ORDER BY id")
	if err != nil {
		return nil, err
	}
	return &ArticleStream{rows: rows, text: s}, nil
}

func (r *ArticleStream) Next() (int64, dict.Entry, error) {
	if !r.rows.Next() {
		if err := r.rows.Err(); err != nil {
			return 0, dict.Entry{}, err
		}
		return 0, dict.Entry{}, io.EOF
	}
	var id int64
	var headword string
	var body []byte
	if err := r.rows.Scan(&id, &headword, &body); err != nil {
		return 0, dict.Entry{}, err
	}
	return id, dict.Entry{Headwords: []string{headword}, Body: r.text.body.decode(body), Kind: dict.BodyHTML}, nil
}

func (r *ArticleStream) Close() error { return r.rows.Close() }

func ReadFavouriteRecords(textDB string) ([]FavouriteRecord, error) {
	db, err := openRO(textDB)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT entry_id,body_size,favourite_id,source_id,source_headword,source_json,assets_json
		FROM favourite_article ORDER BY entry_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FavouriteRecord
	for rows.Next() {
		var record FavouriteRecord
		if err := rows.Scan(&record.EntryID, &record.BodySize, &record.ID, &record.SourceID, &record.SourceHeadword, &record.SourceJSON, &record.AssetsJSON); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func WriteFavouriteRecords(textDB, name, about string, records []FavouriteRecord) error {
	db, err := sql.Open(driverName, "file:"+uriPath(textDB)+"?mode=rw&_busy_timeout=5000"+cacheClause())
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS favourite_article(
		entry_id INTEGER PRIMARY KEY,
		body_size INTEGER NOT NULL DEFAULT 0,
		favourite_id TEXT NOT NULL UNIQUE,
		source_id TEXT NOT NULL,
		source_headword TEXT NOT NULL,
		source_json TEXT NOT NULL,
		assets_json TEXT NOT NULL,
		UNIQUE(source_id,source_headword)
	)`); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM favourite_article"); err != nil {
		return err
	}
	ins, err := tx.Prepare(`INSERT INTO favourite_article(entry_id,body_size,favourite_id,source_id,source_headword,source_json,assets_json)
		VALUES(?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer ins.Close()
	for _, record := range records {
		if _, err := ins.Exec(record.EntryID, record.BodySize, record.ID, record.SourceID, record.SourceHeadword, record.SourceJSON, record.AssetsJSON); err != nil {
			return err
		}
	}
	for key, value := range map[string]string{"format": "favs", "name": name, "description": about, "favourites_version": "1", "favourites_about": about} {
		if _, err := tx.Exec(`INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func SetFavouriteAbout(textDB, about string) error {
	db, err := sql.Open(driverName, "file:"+uriPath(textDB)+"?mode=rw&_busy_timeout=5000"+cacheClause())
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO meta(key,value) VALUES('favourites_about',?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, about)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO meta(key,value) VALUES('description',?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, about)
	return err
}

func SetFavouriteName(textDB, name string) error {
	db, err := sql.Open(driverName, "file:"+uriPath(textDB)+"?mode=rw&_busy_timeout=5000"+cacheClause())
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO meta(key,value) VALUES('name',?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, name)
	return err
}

func (s *Store) FavouriteAbout() string {
	var about string
	_ = s.db.QueryRow("SELECT value FROM meta WHERE key='favourites_about'").Scan(&about)
	return about
}

func (s *Store) FavouriteID(entryID int64) string {
	var id string
	_ = s.db.QueryRow("SELECT favourite_id FROM favourite_article WHERE entry_id=?", entryID).Scan(&id)
	return id
}

func (s *Store) FavouriteSource(entryID int64) *dict.FavoriteSource {
	var raw string
	if err := s.db.QueryRow("SELECT source_json FROM favourite_article WHERE entry_id=?", entryID).Scan(&raw); err != nil {
		return nil
	}
	var stored struct {
		DictionaryID   string `json:"dictionary_id"`
		DictionaryName string `json:"dictionary_name"`
		Headword       string `json:"headword"`
		SourcePath     string `json:"source_path"`
		Headwords      int    `json:"headwords"`
		Articles       int    `json:"articles"`
	}
	if json.Unmarshal([]byte(raw), &stored) != nil {
		return nil
	}
	return &dict.FavoriteSource{
		DictionaryID: stored.DictionaryID, DictionaryName: stored.DictionaryName,
		Headword: stored.Headword, SourcePath: stored.SourcePath,
		Headwords: stored.Headwords, Articles: stored.Articles,
	}
}

func (s *Store) HasFavourite(sourceID, headword string) bool {
	var exists int
	err := s.db.QueryRow("SELECT 1 FROM favourite_article WHERE source_id=? AND source_headword=? LIMIT 1", sourceID, headword).Scan(&exists)
	return err == nil
}

func (s *Store) AssetToken(entryID int64, rawURL string) (string, bool) {
	var data string
	if err := s.db.QueryRow("SELECT assets_json FROM favourite_article WHERE entry_id=?", entryID).Scan(&data); err != nil {
		return "", false
	}
	var assets map[string]FavouriteAsset
	if json.Unmarshal([]byte(data), &assets) != nil {
		return "", false
	}
	if asset, ok := assets[rawURL]; ok {
		return asset.Token, asset.Token != ""
	}
	name := assetLookupName(rawURL)
	for key, asset := range assets {
		if strings.EqualFold(key, name) || strings.EqualFold(asset.Name, name) || strings.EqualFold(assetLookupName(asset.URL), name) {
			return asset.Token, asset.Token != ""
		}
	}
	return "", false
}

func (s *Store) Resources() []string {
	rows, err := s.db.Query("SELECT assets_json FROM favourite_article")
	if err != nil {
		return nil
	}
	defer rows.Close()
	seen := map[string]bool{}
	var names []string
	for rows.Next() {
		var data string
		if rows.Scan(&data) != nil {
			return nil
		}
		var assets map[string]FavouriteAsset
		if json.Unmarshal([]byte(data), &assets) != nil {
			continue
		}
		for _, asset := range assets {
			if asset.Token != "" && !seen[asset.Token] {
				seen[asset.Token] = true
				names = append(names, asset.Token)
			}
		}
	}
	if rows.Err() != nil {
		return nil
	}
	sort.Strings(names)
	return names
}

func (s *Store) decorateFavouriteResults(results []dict.Result) error {
	if len(results) == 0 {
		return nil
	}
	ids := make([]string, len(results))
	args := make([]any, len(results))
	for i, result := range results {
		if result.EntryID <= 0 {
			continue
		}
		ids[i], args[i] = "?", result.EntryID
	}
	for i := range ids {
		if ids[i] == "" {
			ids[i] = "NULL"
			args[i] = nil
		}
	}
	rows, err := s.db.Query(`SELECT entry_id,favourite_id,source_json,assets_json FROM favourite_article WHERE entry_id IN (`+strings.Join(ids, ",")+")", args...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	defer rows.Close()
	type metadata struct {
		id, source, assets string
	}
	byEntry := map[int64]metadata{}
	for rows.Next() {
		var entryID int64
		var m metadata
		if err := rows.Scan(&entryID, &m.id, &m.source, &m.assets); err != nil {
			return err
		}
		byEntry[entryID] = m
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range results {
		m, ok := byEntry[results[i].EntryID]
		if !ok {
			continue
		}
		results[i].FavoriteID = m.id
		results[i].FavoriteSource = favouriteSource(m.source)
		var assets map[string]FavouriteAsset
		if json.Unmarshal([]byte(m.assets), &assets) == nil {
			results[i].ResourceMap = make(map[string]string, len(assets)*2)
			for key, asset := range assets {
				if asset.Token == "" {
					continue
				}
				if key != "" {
					results[i].ResourceMap[strings.ToLower(key)] = asset.Token
				}
				if asset.Name != "" {
					results[i].ResourceMap[strings.ToLower(asset.Name)] = asset.Token
				}
				if asset.URL != "" {
					results[i].ResourceMap[strings.ToLower(asset.URL)] = asset.Token
				}
			}
		}
	}
	return nil
}

func favouriteSource(raw string) *dict.FavoriteSource {
	var stored struct {
		DictionaryID   string `json:"dictionary_id"`
		DictionaryName string `json:"dictionary_name"`
		Headword       string `json:"headword"`
		SourcePath     string `json:"source_path"`
		Headwords      int    `json:"headwords"`
		Articles       int    `json:"articles"`
	}
	if json.Unmarshal([]byte(raw), &stored) != nil {
		return nil
	}
	return &dict.FavoriteSource{
		DictionaryID: stored.DictionaryID, DictionaryName: stored.DictionaryName,
		Headword: stored.Headword, SourcePath: stored.SourcePath,
		Headwords: stored.Headwords, Articles: stored.Articles,
	}
}

func assetLookupName(raw string) string {
	for _, prefix := range []string{"sound://", "file://"} {
		if strings.HasPrefix(strings.ToLower(raw), prefix) {
			raw = raw[len(prefix):]
			break
		}
	}
	raw = strings.TrimPrefix(raw, "./")
	raw = strings.TrimPrefix(raw, "/")
	return strings.ToLower(raw)
}

func ValidateFavouriteRecords(records []FavouriteRecord) error {
	ids, keys := map[string]bool{}, map[string]bool{}
	for _, record := range records {
		if record.ID == "" || record.EntryID <= 0 || record.SourceID == "" || record.SourceHeadword == "" {
			return fmt.Errorf("invalid favourites article metadata")
		}
		if ids[record.ID] || keys[record.SourceID+"\x00"+record.SourceHeadword] {
			return fmt.Errorf("duplicate favourites article metadata")
		}
		ids[record.ID], keys[record.SourceID+"\x00"+record.SourceHeadword] = true, true
	}
	return nil
}
