package database

import (
	"database/sql"

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
		processed_image BLOB
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

func (s *SQLiteDatabase) GetAllImages() ([]*Image, error) {
	// Ensure deterministic ordering. Without ORDER BY, SQLite does not guarantee row order.
	// Use rowid as a stable insertion order proxy since schema has no created_at.
	rows, err := s.db.Query("SELECT id, original_image, processed_image FROM images ORDER BY rowid ASC")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close() // Explicitly ignore error as we're already returning an error from the function
	}()

	var images []*Image
	for rows.Next() {
		var img Image
		if err := rows.Scan(&img.ID, &img.OriginalImage, &img.ProcessedImage); err != nil {
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
	if processed == nil || len(processed) == 0 {
		return nil, sql.ErrNoRows
	}
	return processed, nil
}
