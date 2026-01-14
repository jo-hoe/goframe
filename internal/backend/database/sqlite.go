package database

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"

	_ "modernc.org/sqlite"
)

type SQLiteDatabase struct {
	db               *sql.DB
	connectionString string
}

func NewSQLiteDatabase(connectionString string) (DatabaseService, error) {
	db, err := sql.Open("sqlite", connectionString)
	if err != nil {
		return nil, err
	}

	// Ensure a single underlying connection to avoid issues with in-memory or per-connection state.
	// This also stabilizes behavior in environments where ':memory:' might still be configured.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	return &SQLiteDatabase{
		db:               db,
		connectionString: connectionString,
	}, nil
}

func (s *SQLiteDatabase) CreateDatabase() (*sql.DB, error) {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS images (
		id TEXT PRIMARY KEY,
		original_image BLOB,
		processed_image BLOB,
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
	)`)
	if err != nil {
		return nil, err
	}

	return s.db, nil
}

func (s *SQLiteDatabase) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func (s *SQLiteDatabase) DoesDatabaseExist() bool {
	// In SQLite, the database file is created when you connect to it.
	// So we can assume it exists if we can successfully ping the database.
	err := s.db.Ping()
	return err == nil
}

func (s *SQLiteDatabase) CreateImage(original []byte, processed []byte) (string, error) {
	id, err := generateID(original)
	if err != nil {
		return "", err
	}

	// Insert both original and processed image atomically to avoid NULL race windows
	_, err = s.db.Exec("INSERT INTO images (id, original_image, processed_image) VALUES (?, ?, ?)", id, original, processed)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *SQLiteDatabase) SetProcessedImage(id string, processedImage []byte) error {
	_, err := s.db.Exec("UPDATE images SET processed_image = ? WHERE id = ?", processedImage, id)
	return err
}

func (s *SQLiteDatabase) GetImages(fields ...string) ([]*Image, error) {
	// Build mapping from db tag -> struct field index dynamically from Image type
	imgType := reflect.TypeOf(Image{})
	tagToIndex := make(map[string]int, imgType.NumField())
	allTags := make([]string, 0, imgType.NumField())
	for i := 0; i < imgType.NumField(); i++ {
		f := imgType.Field(i)
		tag := f.Tag.Get("db")
		if tag == "" {
			continue
		}
		tagToIndex[tag] = i
		allTags = append(allTags, tag)
	}

	selected := fields
	if len(selected) == 0 {
		selected = allTags
	} else {
		// Validate the requested fields exist on the Image struct tags
		for _, fld := range selected {
			if _, ok := tagToIndex[fld]; !ok {
				return nil, fmt.Errorf("unknown image field %q", fld)
			}
		}
	}

	selectClause := strings.Join(selected, ", ")
	query := fmt.Sprintf("SELECT %s FROM images ORDER BY created_at ASC, rowid ASC", selectClause)

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var images []*Image
	for rows.Next() {
		var img Image
		v := reflect.ValueOf(&img).Elem()

		// Build scan destinations to the requested struct field pointers
		dest := make([]any, 0, len(selected))
		for _, tag := range selected {
			idx := tagToIndex[tag]
			field := v.Field(idx)
			dest = append(dest, field.Addr().Interface())
		}

		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		images = append(images, &img)
	}
	return images, nil
}

func (s *SQLiteDatabase) DeleteImage(id string) error {
	_, err := s.db.Exec("DELETE FROM images WHERE id = ?", id)
	return err
}

func (s *SQLiteDatabase) GetOriginalImageByID(id string) ([]byte, error) {
	row := s.db.QueryRow("SELECT original_image FROM images WHERE id = ?", id)
	var original []byte
	if err := row.Scan(&original); err != nil {
		return nil, err
	}
	return original, nil
}

func (s *SQLiteDatabase) GetProcessedImageByID(id string) ([]byte, error) {
	row := s.db.QueryRow("SELECT processed_image FROM images WHERE id = ?", id)
	var processed []byte
	if err := row.Scan(&processed); err != nil {
		return nil, err
	}
	// Guard against race where the row exists but processed_image is still NULL
	if len(processed) == 0 {
		return nil, sql.ErrNoRows
	}
	return processed, nil
}
