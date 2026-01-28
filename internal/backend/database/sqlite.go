package database

import (
	"database/sql"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteDatabase struct {
	db               *sql.DB
	connectionString string

	// Prepared statements for common operations
	insertStmt          *sql.Stmt
	updateProcessedStmt *sql.Stmt
	deleteStmt          *sql.Stmt
	getByIDStmt         *sql.Stmt
}

func NewSQLiteDatabase(connectionString string) (DatabaseService, error) {
	db, err := sql.Open("sqlite", connectionString)
	if err != nil {
		return nil, err
	}

	// Enable WAL mode for better concurrency and set a busy timeout to mitigate lock contention.
	// Ignore errors here to avoid failing initialization on environments that restrict PRAGMA changes.
	_, _ = db.Exec(`PRAGMA journal_mode=WAL;`)
	_, _ = db.Exec(`PRAGMA busy_timeout=3000;`) // 3s; adjust if needed

	// Pool sizing:
	// - For in-memory databases (":memory:"), keep a single connection to avoid separate DBs per connection.
	// - For file-based databases, allow multiple connections based on GOMAXPROCS.
	if strings.Contains(strings.ToLower(connectionString), ":memory:") {
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
	} else {
		max := runtime.GOMAXPROCS(0) * 2
		if max < 4 {
			max = 4
		}
		db.SetMaxOpenConns(max)
		db.SetMaxIdleConns(max)
	}

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
		rank TEXT NOT NULL
	)`)
	if err != nil {
		return nil, err
	}

	// Prepare common statements for reuse under load
	if s.insertStmt, err = s.db.Prepare(`INSERT INTO images (id, original_image, processed_image, rank) VALUES (?, ?, ?, ?)`); err != nil {
		return nil, err
	}
	if s.updateProcessedStmt, err = s.db.Prepare(`UPDATE images SET processed_image = ? WHERE id = ?`); err != nil {
		return nil, err
	}
	if s.deleteStmt, err = s.db.Prepare(`DELETE FROM images WHERE id = ?`); err != nil {
		return nil, err
	}
	if s.getByIDStmt, err = s.db.Prepare(`SELECT id, original_image, processed_image, rank FROM images WHERE id = ?`); err != nil {
		return nil, err
	}

	return s.db, nil
}

func (s *SQLiteDatabase) Close() error {
	var firstErr error
	// Close prepared statements
	if s.insertStmt != nil {
		if err := s.insertStmt.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if s.updateProcessedStmt != nil {
		if err := s.updateProcessedStmt.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if s.deleteStmt != nil {
		if err := s.deleteStmt.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if s.getByIDStmt != nil {
		if err := s.getByIDStmt.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	if s.db != nil {
		if err := s.db.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *SQLiteDatabase) DoesDatabaseExist() bool {
	// In SQLite, the database file is created when you connect to it.
	// So we can assume it exists if we can successfully ping the database.
	err := s.db.Ping()
	return err == nil
}

func (s *SQLiteDatabase) CreateImage(original []byte, processed []byte) (string, error) {
	if original == nil {
		return "", fmt.Errorf("original image data cannot be nil")
	}
	if processed == nil {
		return "", fmt.Errorf("processed image data cannot be nil")
	}

	id, err := generateID()
	if err != nil {
		return "", err
	}

	// Determine next LexoRank at end of list
	var lastRank sql.NullString
	if err := s.db.QueryRow("SELECT rank FROM images ORDER BY rank DESC, rowid DESC LIMIT 1").Scan(&lastRank); err != nil && err != sql.ErrNoRows {
		return "", err
	}
	newRank := nextRank("")
	if lastRank.Valid {
		newRank = nextRank(lastRank.String)
	}

	// Insert both original and processed image atomically to avoid NULL race windows, with computed rank
	if s.insertStmt != nil {
		_, err = s.insertStmt.Exec(id, original, processed, newRank)
	} else {
		_, err = s.db.Exec("INSERT INTO images (id, original_image, processed_image, rank) VALUES (?, ?, ?, ?)", id, original, processed, newRank)
	}
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *SQLiteDatabase) SetProcessedImage(id string, processedImage []byte) error {
	if s.updateProcessedStmt != nil {
		_, err := s.updateProcessedStmt.Exec(processedImage, id)
		return err
	}
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
	query := fmt.Sprintf("SELECT %s FROM images ORDER BY rank ASC, rowid ASC", selectClause)

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

		// For fields of type time.Time, scan into temporary strings and parse after Scan
		stringHolders := make([]string, len(selected))

		// Build scan destinations to the requested struct field pointers
		dest := make([]any, 0, len(selected))
		for i, tag := range selected {
			idx := tagToIndex[tag]
			field := v.Field(idx)
			if field.Type() == reflect.TypeOf(time.Time{}) {
				// Scan TEXT into a temporary string, then parse into time.Time after Scan
				dest = append(dest, &stringHolders[i])
			} else {
				dest = append(dest, field.Addr().Interface())
			}
		}

		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}

		// Parse any time fields from their temporary string holders
		for i, tag := range selected {
			idx := tagToIndex[tag]
			field := v.Field(idx)
			if field.Type() == reflect.TypeOf(time.Time{}) {
				val := stringHolders[i]
				if val != "" {
					// created_at format: strftime('%Y-%m-%dT%H:%M:%fZ','now'); parse using RFC3339Nano to handle variable fractional seconds
					tm, parseErr := time.Parse(time.RFC3339Nano, val)
					if parseErr != nil {
						return nil, fmt.Errorf("failed to parse time for field %q: %w", tag, parseErr)
					}
					field.Set(reflect.ValueOf(tm))
				} else {
					// zero value if empty
					field.Set(reflect.ValueOf(time.Time{}))
				}
			}
		}

		images = append(images, &img)
	}
	return images, nil
}

func (s *SQLiteDatabase) DeleteImage(id string) error {
	if s.deleteStmt != nil {
		_, err := s.deleteStmt.Exec(id)
		return err
	}
	_, err := s.db.Exec("DELETE FROM images WHERE id = ?", id)
	return err
}

func (s *SQLiteDatabase) GetImageByID(id string) (*Image, error) {
	var row *sql.Row
	if s.getByIDStmt != nil {
		row = s.getByIDStmt.QueryRow(id)
	} else {
		row = s.db.QueryRow("SELECT id, original_image, processed_image, rank FROM images WHERE id = ?", id)
	}

	var img Image
	var rankStr string
	if err := row.Scan(&img.ID, &img.OriginalImage, &img.ProcessedImage, &rankStr); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Not found
		}
		return nil, err
	}

	if rankStr != "" {
		img.Rank = rankStr
	}

	return &img, nil
}
