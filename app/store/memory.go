package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Memories retrieves all persistent memories
func (s *Store) Memories() ([]Memory, error) {
	if err := s.ensureDB(); err != nil {
		return nil, err
	}

	rows, err := s.db.conn.Query("SELECT id, title, content, COALESCE(source_chat_id, ''), created_at, updated_at FROM memories ORDER BY created_at DESC")
	if err != nil {
		return nil, fmt.Errorf("query memories: %w", err)
	}
	defer rows.Close()

	var memories []Memory
	for rows.Next() {
		var m Memory
		if err := rows.Scan(&m.ID, &m.Title, &m.Content, &m.SourceChatID, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan memory: %w", err)
		}
		memories = append(memories, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memories iteration: %w", err)
	}

	return memories, nil
}

// Memory retrieves a specific memory by ID
func (s *Store) Memory(id string) (*Memory, error) {
	if err := s.ensureDB(); err != nil {
		return nil, err
	}

	var m Memory
	err := s.db.conn.QueryRow("SELECT id, title, content, COALESCE(source_chat_id, ''), created_at, updated_at FROM memories WHERE id = ?", id).
		Scan(&m.ID, &m.Title, &m.Content, &m.SourceChatID, &m.CreatedAt, &m.UpdatedAt)
	
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get memory: %w", err)
	}

	return &m, nil
}

// CreateMemory inserts a new memory
func (s *Store) CreateMemory(m Memory) error {
	if err := s.ensureDB(); err != nil {
		return err
	}

	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = time.Now()
	}

	_, err := s.db.conn.Exec(`
		INSERT INTO memories (id, title, content, source_chat_id, created_at, updated_at) 
		VALUES (?, ?, ?, ?, ?, ?)`,
		m.ID, m.Title, m.Content, m.SourceChatID, m.CreatedAt, m.UpdatedAt)
	
	if err != nil {
		return fmt.Errorf("insert memory: %w", err)
	}
	return nil
}

// UpdateMemory updates an existing memory
func (s *Store) UpdateMemory(m Memory) error {
	if err := s.ensureDB(); err != nil {
		return err
	}

	m.UpdatedAt = time.Now()

	res, err := s.db.conn.Exec(`
		UPDATE memories 
		SET title = ?, content = ?, source_chat_id = ?, updated_at = ?
		WHERE id = ?`,
		m.Title, m.Content, m.SourceChatID, m.UpdatedAt, m.ID)
	
	if err != nil {
		return fmt.Errorf("update memory: %w", err)
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("memory not found: %s", m.ID)
	}

	return nil
}

// DeleteMemory deletes a memory by ID
func (s *Store) DeleteMemory(id string) error {
	if err := s.ensureDB(); err != nil {
		return err
	}

	_, err := s.db.conn.Exec("DELETE FROM memories WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete memory: %w", err)
	}
	return nil
}
