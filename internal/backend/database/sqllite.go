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

	return &SQLiteDatabase{
		db:               db,
		connectionString: connectionString,
	}, nil
}

func (s *SQLiteDatabase) CreateDatabase() (*sql.DB, error) {
	s.db.Exec(`CREATE TABLE IF NOT EXISTS entries (
		id TEXT PRIMARY KEY,
		original_image BLOB,
		processed_image BLOB
	)`)

	return s.db, nil
}

func (s *SQLiteDatabase) DoesDatabaseExist() bool {
	// In SQLite, the database file is created when you connect to it.
	// So we can assume it exists if we can successfully ping the database.
	err := s.db.Ping()
	return err == nil
}

func (s *SQLiteDatabase) CreateEntry(image []byte) (*Entry, error) {
	id, err := generateID(image)
	if err != nil {
		return nil, err
	}
	entry := &Entry{
		ID:            id,
		OriginalImage: image,
	}

	_, err = s.db.Exec("INSERT INTO entries (id, original_image, processed_image) VALUES (?, ?, ?)", entry.ID, entry.OriginalImage, nil)
	if err != nil {
		return nil, err
	}

	return entry, nil
}

func (s *SQLiteDatabase) AddProccessedImage(id string, processedImage []byte) error {
	_, err := s.db.Exec("UPDATE entries SET processed_image = ? WHERE id = ?", processedImage, id)
	return err
}

func (s *SQLiteDatabase) GetAllEntrys() ([]*Entry, error) {
	rows, err := s.db.Query("SELECT id, original_image, processed_image FROM entries")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*Entry
	for rows.Next() {
		var entry Entry
		if err := rows.Scan(&entry.ID, &entry.OriginalImage, &entry.ProcessedImage); err != nil {
			return nil, err
		}
		entries = append(entries, &entry)
	}
	return entries, nil
}

func (s *SQLiteDatabase) DeleteEntry(id string) error {
	_, err := s.db.Exec("DELETE FROM entries WHERE id = ?", id)
	return err
}

func (s *SQLiteDatabase) GetEntryByID(id string) (*Entry, error) {
	row := s.db.QueryRow("SELECT id, original_image, processed_image FROM entries WHERE id = ?", id)
	var entry Entry
	if err := row.Scan(&entry.ID, &entry.OriginalImage, &entry.ProcessedImage); err != nil {
		return nil, err
	}
	return &entry, nil
}
