package database

import (
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"

	_ "modernc.org/sqlite"
)

type SQLiteDatabase struct {
	db               *sql.DB
	connectionString string

	// Prepared statements for common operations
	insertStmt  *sql.Stmt
	deleteStmt  *sql.Stmt
	getByIDStmt *sql.Stmt
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

func (s *SQLiteDatabase) CreateDatabase() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS images (
		id TEXT PRIMARY KEY,
		original_image BLOB,
		processed_image BLOB,
		rank TEXT NOT NULL
	)`)
	if err != nil {
		return err
	}

	// Prepare common statements for reuse under load
	if s.insertStmt, err = s.db.Prepare(`INSERT INTO images (id, original_image, processed_image, rank) VALUES (?, ?, ?, ?)`); err != nil {
		return err
	}
	if s.deleteStmt, err = s.db.Prepare(`DELETE FROM images WHERE id = ?`); err != nil {
		return err
	}
	if s.getByIDStmt, err = s.db.Prepare(`SELECT id, original_image, processed_image, rank FROM images WHERE id = ?`); err != nil {
		return err
	}

	return nil
}

func (s *SQLiteDatabase) Close() error {
	var errs []error

	// Close prepared statements, collect all errors
	if s.insertStmt != nil {
		if err := s.insertStmt.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.deleteStmt != nil {
		if err := s.deleteStmt.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.getByIDStmt != nil {
		if err := s.getByIDStmt.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	// Close DB connection last
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	// Join all errors (returns nil if slice is empty)
	return errors.Join(errs...)
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
	prev := ""
	if lastRank.Valid {
		prev = lastRank.String
	}
	newRank := Next(prev)

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
	// #nosec G201 -- selectClause is constructed from validated struct tags (whitelisted), preventing injection
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

func (s *SQLiteDatabase) UpdateRanks(order []string) error {
	if len(order) == 0 {
		return nil
	}

	// Load existing ranks for specified IDs
	placeholders := strings.TrimRight(strings.Repeat("?,", len(order)), ",")
	args := make([]any, 0, len(order))
	for _, id := range order {
		args = append(args, id)
	}

	// #nosec G202 -- placeholders are statically generated as "?,...?" and args are bound parameters; no string interpolation of values
	rows, err := s.db.Query("SELECT id, rank FROM images WHERE id IN ("+placeholders+")", args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	existing := make(map[string]string, len(order))
	for rows.Next() {
		var id string
		var rank sql.NullString
		if err := rows.Scan(&id, &rank); err != nil {
			return err
		}
		if rank.Valid {
			existing[id] = rank.String
		} else {
			existing[id] = ""
		}
	}

	updates := Reorder(existing, order)
	if len(updates) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	// Always rollback; if Commit succeeds later this will return sql.ErrTxDone and be ignored
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare("UPDATE images SET rank = ? WHERE id = ?")
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()

	for id, newRank := range updates {
		if _, err = stmt.Exec(newRank, id); err != nil {
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}
