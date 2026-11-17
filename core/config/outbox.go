package config

import "errors"

func normalizeOutboxProperties(properties *OutboxProperties) error {
	if properties.MaxPending < 0 || properties.MaxEntries < 0 || properties.MaxBytes < 0 || properties.RetainDone < 0 {
		return errors.New("persistence.outbox limits must be non-negative")
	}
	if properties.MaxPending == 0 {
		properties.MaxPending = 10000
	}
	if properties.MaxEntries == 0 {
		properties.MaxEntries = 20000
	}
	if properties.MaxBytes == 0 {
		properties.MaxBytes = 64 * 1024 * 1024
	}
	if properties.RetainDone == 0 {
		properties.RetainDone = 10000
	}
	if properties.MaxBytes < 65536 {
		return errors.New("persistence.outbox.max_bytes must be at least 65536")
	}
	if properties.MaxEntries < properties.MaxPending {
		return errors.New("persistence.outbox.max_entries must not be less than max_pending")
	}
	if properties.RetainDone > properties.MaxEntries {
		return errors.New("persistence.outbox.retain_done must not exceed max_entries")
	}
	return nil
}
