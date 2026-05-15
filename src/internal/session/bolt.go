package session

import (
	"fmt"
	"net"

	"go.etcd.io/bbolt"
	"go.uber.org/zap"
)

// @sk-task production-hardening#T3.1: bolt db store (AC-006)
// @sk-task production-readiness-hardening#T1.1: add logger DI (AC-006)
type BoltStore struct {
	db     *bbolt.DB
	bucket []byte
	logger *zap.Logger
}

// @sk-task production-readiness-hardening#T1.1: add logger DI (AC-006)
func NewBoltStore(path string, logger *zap.Logger) (*BoltStore, error) {
	db, err := bbolt.Open(path, 0o600, nil)
	if err != nil {
		return nil, fmt.Errorf("open bolt db %s: %w", path, err)
	}
	s := &BoltStore{db: db, bucket: []byte("allocations"), logger: logger}
	if err := s.db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(s.bucket)
		return err
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create bucket: %w", err)
	}
	return s, nil
}

func (s *BoltStore) Close() error {
	return s.db.Close()
}

// @sk-task ipv6-dual-stack#T1.2: bolt db store for IPv6 allocations (AC-002)
// @sk-task production-readiness-hardening#T1.1: add logger DI (AC-006)
func NewBoltStore6(path string, logger *zap.Logger) (*BoltStore, error) {
	db, err := bbolt.Open(path, 0o600, nil)
	if err != nil {
		return nil, fmt.Errorf("open bolt db %s: %w", path, err)
	}
	s := &BoltStore{db: db, bucket: []byte("allocations_v6"), logger: logger}
	if err := s.db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(s.bucket)
		return err
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create bucket: %w", err)
	}
	return s, nil
}

// @sk-task production-hardening#T3.1: save allocations to bolt (AC-006)
func (s *BoltStore) SaveAllocations(allocated map[string]net.IP) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(s.bucket)
		if b == nil {
			return fmt.Errorf("bucket %q not found", string(s.bucket))
		}
		for sessionID, ip := range allocated {
			if err := b.Put([]byte(sessionID), []byte(ip.String())); err != nil {
				return err
			}
		}
		return nil
	})
}

// @sk-task production-hardening#T3.1: load allocations from bolt (AC-006)
func (s *BoltStore) LoadAllocations() (map[string]net.IP, error) {
	result := make(map[string]net.IP)
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(s.bucket)
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			ip := net.ParseIP(string(v))
			if ip == nil {
				s.logger.Warn("invalid IP in bolt store", zap.String("ip", string(v)), zap.String("key", string(k)))
				return nil
			}
			result[string(k)] = ip
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
