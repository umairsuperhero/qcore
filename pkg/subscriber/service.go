package subscriber

import (
	"context"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/metrics"
	"gorm.io/gorm"
)

type Service struct {
	db      *gorm.DB
	log     logger.Logger
	metrics *metrics.HSSMetrics
	plmnID  [3]byte
}

func NewService(db *gorm.DB, log logger.Logger, m *metrics.HSSMetrics, plmnID [3]byte) *Service {
	return &Service{
		db:      db,
		log:     log.WithField("component", "subscriber"),
		metrics: m,
		plmnID:  plmnID,
	}
}

func (s *Service) CreateSubscriber(ctx context.Context, sub *Subscriber) error {
	if sub.AMF == "" {
		sub.AMF = "8000"
	}
	if sub.SQN == "" {
		sub.SQN = "000000000000"
	}
	if sub.APN == "" {
		sub.APN = "internet"
	}

	if err := sub.Validate(); err != nil {
		return fmt.Errorf("validation: %w", err)
	}

	if err := s.db.WithContext(ctx).Create(sub).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "UNIQUE") {
			return fmt.Errorf("subscriber with IMSI %s already exists", sub.IMSI)
		}
		return fmt.Errorf("creating subscriber: %w", err)
	}

	if s.metrics != nil {
		s.metrics.SubscriberTotal.WithLabelValues().Inc()
	}
	s.log.Infof("Created subscriber IMSI=%s", sub.IMSI)
	return nil
}

func (s *Service) GetSubscriber(ctx context.Context, imsi string) (*Subscriber, error) {
	var sub Subscriber
	if err := s.db.WithContext(ctx).Where("imsi = ?", imsi).First(&sub).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("subscriber %s not found", imsi)
		}
		return nil, fmt.Errorf("querying subscriber: %w", err)
	}
	return &sub, nil
}

func (s *Service) UpdateSubscriber(ctx context.Context, imsi string, updates *Subscriber) error {
	result := s.db.WithContext(ctx).Model(&Subscriber{}).Where("imsi = ?", imsi).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("updating subscriber: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("subscriber %s not found", imsi)
	}
	s.log.Infof("Updated subscriber IMSI=%s", imsi)
	return nil
}

func (s *Service) DeleteSubscriber(ctx context.Context, imsi string) error {
	result := s.db.WithContext(ctx).Where("imsi = ?", imsi).Delete(&Subscriber{})
	if result.Error != nil {
		return fmt.Errorf("deleting subscriber: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("subscriber %s not found", imsi)
	}
	if s.metrics != nil {
		s.metrics.SubscriberTotal.WithLabelValues().Dec()
	}
	s.log.Infof("Deleted subscriber IMSI=%s", imsi)
	return nil
}

func (s *Service) ListSubscribers(ctx context.Context, page, limit int, search string) ([]Subscriber, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	var total int64
	query := s.db.WithContext(ctx).Model(&Subscriber{})

	if search != "" {
		like := "%" + search + "%"
		query = query.Where("imsi LIKE ? OR msisdn LIKE ?", like, like)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("counting subscribers: %w", err)
	}

	var subscribers []Subscriber
	offset := (page - 1) * limit
	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&subscribers).Error; err != nil {
		return nil, 0, fmt.Errorf("listing subscribers: %w", err)
	}

	return subscribers, total, nil
}

func (s *Service) GenerateAuthVector(ctx context.Context, imsi string) (*AuthVector, error) {
	sub, err := s.GetSubscriber(ctx, imsi)
	if err != nil {
		return nil, err
	}

	ki, err := sub.KiBytes()
	if err != nil {
		return nil, fmt.Errorf("decoding Ki: %w", err)
	}
	opc, err := sub.OPcBytes()
	if err != nil {
		return nil, fmt.Errorf("decoding OPc: %w", err)
	}
	sqn, err := sub.SQNBytes()
	if err != nil {
		return nil, fmt.Errorf("decoding SQN: %w", err)
	}
	amf, err := sub.AMFBytes()
	if err != nil {
		return nil, fmt.Errorf("decoding AMF: %w", err)
	}

	av, err := GenerateAuthVector(ki, opc, sqn, amf, s.plmnID)
	if err != nil {
		return nil, fmt.Errorf("generating auth vector: %w", err)
	}

	if err := sub.IncrementSQN(); err != nil {
		return nil, fmt.Errorf("incrementing SQN: %w", err)
	}
	if err := s.db.WithContext(ctx).Model(sub).Update("sqn", sub.SQN).Error; err != nil {
		return nil, fmt.Errorf("saving SQN: %w", err)
	}

	if s.metrics != nil {
		s.metrics.AuthVectors.WithLabelValues().Inc()
	}
	s.log.Infof("Generated auth vector for IMSI=%s", imsi)
	return av, nil
}

// Generate5GAuthVector issues a TS 33.501 Annex A 5G-AKA vector for the
// given IMSI and serving-network name. Shares the per-subscriber SQN
// counter with the 4G path — both EPS-AKA and 5G-AKA advance the same
// SQN, which matches what happens in deployed cores (a subscriber has
// one authentication identity, not two).
func (s *Service) Generate5GAuthVector(ctx context.Context, imsi, snName string) (*AuthVector5G, error) {
	sub, err := s.GetSubscriber(ctx, imsi)
	if err != nil {
		return nil, err
	}

	ki, err := sub.KiBytes()
	if err != nil {
		return nil, fmt.Errorf("decoding Ki: %w", err)
	}
	opc, err := sub.OPcBytes()
	if err != nil {
		return nil, fmt.Errorf("decoding OPc: %w", err)
	}
	sqn, err := sub.SQNBytes()
	if err != nil {
		return nil, fmt.Errorf("decoding SQN: %w", err)
	}
	amf, err := sub.AMFBytes()
	if err != nil {
		return nil, fmt.Errorf("decoding AMF: %w", err)
	}

	av, err := Generate5GAuthVector(ki, opc, sqn, amf, snName)
	if err != nil {
		return nil, fmt.Errorf("generating 5G auth vector: %w", err)
	}

	if err := sub.IncrementSQN(); err != nil {
		return nil, fmt.Errorf("incrementing SQN: %w", err)
	}
	if err := s.db.WithContext(ctx).Model(sub).Update("sqn", sub.SQN).Error; err != nil {
		return nil, fmt.Errorf("saving SQN: %w", err)
	}

	if s.metrics != nil {
		s.metrics.AuthVectors.WithLabelValues().Inc()
	}
	s.log.Infof("Generated 5G auth vector for IMSI=%s sn=%s", imsi, snName)
	return av, nil
}

func (s *Service) ImportCSV(ctx context.Context, reader io.Reader) (int, error) {
	r := csv.NewReader(reader)

	header, err := r.Read()
	if err != nil {
		return 0, fmt.Errorf("reading CSV header: %w", err)
	}

	colMap := make(map[string]int)
	for i, col := range header {
		colMap[strings.ToLower(strings.TrimSpace(col))] = i
	}

	required := []string{"imsi", "ki", "opc"}
	for _, col := range required {
		if _, ok := colMap[col]; !ok {
			return 0, fmt.Errorf("missing required column: %s", col)
		}
	}

	count := 0
	tx := s.db.WithContext(ctx).Begin()

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			tx.Rollback()
			return 0, fmt.Errorf("reading CSV row %d: %w", count+2, err)
		}

		sub := &Subscriber{
			IMSI: record[colMap["imsi"]],
			Ki:   record[colMap["ki"]],
			OPc:  record[colMap["opc"]],
			AMF:  "8000",
			SQN:  "000000000000",
			APN:  "internet",
		}

		if idx, ok := colMap["msisdn"]; ok && idx < len(record) {
			sub.MSISDN = record[idx]
		}
		if idx, ok := colMap["amf"]; ok && idx < len(record) && record[idx] != "" {
			sub.AMF = record[idx]
		}
		if idx, ok := colMap["apn"]; ok && idx < len(record) && record[idx] != "" {
			sub.APN = record[idx]
		}

		if err := sub.Validate(); err != nil {
			tx.Rollback()
			return 0, fmt.Errorf("row %d: %w", count+2, err)
		}

		if err := tx.Create(sub).Error; err != nil {
			tx.Rollback()
			return 0, fmt.Errorf("inserting row %d: %w", count+2, err)
		}
		count++
	}

	if err := tx.Commit().Error; err != nil {
		return 0, fmt.Errorf("committing import: %w", err)
	}

	s.log.Infof("Imported %d subscribers from CSV", count)
	return count, nil
}

func (s *Service) ExportCSV(ctx context.Context, writer io.Writer) error {
	w := csv.NewWriter(writer)
	defer w.Flush()

	if err := w.Write([]string{"imsi", "msisdn", "ki", "opc", "amf", "sqn", "apn"}); err != nil {
		return fmt.Errorf("writing CSV header: %w", err)
	}

	var subscribers []Subscriber
	if err := s.db.WithContext(ctx).Find(&subscribers).Error; err != nil {
		return fmt.Errorf("querying subscribers: %w", err)
	}

	for _, sub := range subscribers {
		if err := w.Write([]string{sub.IMSI, sub.MSISDN, sub.Ki, sub.OPc, sub.AMF, sub.SQN, sub.APN}); err != nil {
			return fmt.Errorf("writing CSV row: %w", err)
		}
	}

	return nil
}

// SetSQN overwrites the stored SQN for an IMSI. Used by UDR when a UEAU
// backend elsewhere (e.g. a UDR-backed UDM AuthSource) has advanced the
// counter and needs to persist it. Validates the hex shape but does not
// enforce monotonicity — replay protection is a caller-side concern.
func (s *Service) SetSQN(ctx context.Context, imsi, newSQN string) error {
	if len(newSQN) != 12 {
		return fmt.Errorf("SQN must be 12 hex chars, got %d", len(newSQN))
	}
	if _, err := hex.DecodeString(newSQN); err != nil {
		return fmt.Errorf("SQN must be hex: %w", err)
	}
	result := s.db.WithContext(ctx).Model(&Subscriber{}).Where("imsi = ?", imsi).Update("sqn", newSQN)
	if result.Error != nil {
		return fmt.Errorf("updating SQN: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("subscriber %s not found", imsi)
	}
	return nil
}

// ParsePLMN converts a PLMN string like "00101" to a 3-byte array.
func ParsePLMN(plmn string) ([3]byte, error) {
	if len(plmn) != 5 && len(plmn) != 6 {
		return [3]byte{}, fmt.Errorf("PLMN must be 5 or 6 digits (MCC+MNC), got %q", plmn)
	}
	b, err := hex.DecodeString(plmn + strings.Repeat("0", 6-len(plmn)))
	if err != nil {
		return [3]byte{}, fmt.Errorf("decoding PLMN: %w", err)
	}
	var result [3]byte
	copy(result[:], b[:3])
	return result, nil
}
